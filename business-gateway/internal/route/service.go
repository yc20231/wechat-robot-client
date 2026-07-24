package route

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"business-gateway/internal/admin"
	"business-gateway/internal/audit"
	"business-gateway/internal/backend"
	"business-gateway/internal/dedup"
	"business-gateway/internal/group"
)

const (
	ModuleStable       = "stable"
	ModuleExperimental = "experimental"
)

var adminInventoryPattern = regexp.MustCompile(`^查\s*([^\s]+)\s*库存(?:\s+(.+))?$`)

type Request struct {
	RobotWxID      string   `json:"robot_wxid"`
	RobotCode      string   `json:"robot_code,omitempty"`
	GroupID        string   `json:"group_id"`
	SenderWxID     string   `json:"sender_wxid"`
	MessageID      int64    `json:"message_id"`
	Content        string   `json:"content"`
	IsAtMe         bool     `json:"is_at_me"`
	MentionedWxIDs []string `json:"mentioned_wxids,omitempty"`
}

type Response struct {
	Handled      bool     `json:"handled"`
	Reply        string   `json:"reply,omitempty"`
	Error        string   `json:"error,omitempty"`
	ReplyAtWxIDs []string `json:"reply_at_wxids,omitempty"`
}

type command struct {
	module       string
	stability    string
	keyword      string
	customerCode string
}

type Service struct {
	groups           group.Store
	backend          backend.Service
	dedup            dedup.Cache
	admins           admin.Store
	audit            audit.Logger
	confirmations    *confirmationStore
	confirmationTTL  time.Duration
	managementMu     sync.Mutex
	requireAtMention bool
}

func NewService(groups group.Store, backendService backend.Service, cache dedup.Cache, admins admin.Store, auditLogger audit.Logger, requireAtMention bool, confirmationTTL time.Duration) *Service {
	if confirmationTTL <= 0 {
		confirmationTTL = 5 * time.Minute
	}
	return &Service{
		groups:           groups,
		backend:          backendService,
		dedup:            cache,
		admins:           admins,
		audit:            auditLogger,
		confirmations:    newConfirmationStore(),
		confirmationTTL:  confirmationTTL,
		requireAtMention: requireAtMention,
	}
}

func (s *Service) Route(ctx context.Context, req Request) Response {
	req.GroupID = strings.TrimSpace(req.GroupID)
	req.SenderWxID = strings.TrimSpace(req.SenderWxID)
	req.RobotWxID = strings.TrimSpace(req.RobotWxID)
	content := stripLeadingMentions(req.Content)
	management, isManagement := parseManagementCommand(content)
	cmd, isBusiness := parseCommand(content)
	if !isManagement && !isBusiness {
		return Response{Handled: false}
	}
	if req.GroupID == "" || req.SenderWxID == "" {
		return businessError("业务消息缺少群或发送者信息")
	}
	if req.RobotWxID != "" && req.SenderWxID == req.RobotWxID {
		return Response{Handled: false}
	}
	if s.requireAtMention && !req.IsAtMe {
		return Response{Handled: false}
	}
	if req.MessageID != 0 && s.dedup.Seen(fmt.Sprintf("route:%s:%d", req.RobotWxID, req.MessageID)) {
		return Response{Handled: true}
	}
	if isManagement {
		return s.handleManagement(ctx, req, management)
	}

	binding, found := s.groups.Get(req.GroupID)
	if !found || !binding.Enabled {
		return businessError("该群尚未配置业务查询，请联系管理员")
	}
	isAdmin := binding.Type == group.TypeAdmin
	adminAllowed := s.admins != nil && s.admins.IsAdmin(req.SenderWxID)
	if isAdmin && cmd.module != "help" && !adminAllowed {
		return businessError("当前账号没有管理员业务查询权限")
	}
	if cmd.stability == ModuleExperimental && (!isAdmin || !adminAllowed) {
		return businessError("该功能目前仅对管理员开放")
	}

	switch cmd.module {
	case "help":
		return Response{Handled: true, Reply: renderHelp(isAdmin && adminAllowed)}
	case "status":
		if err := s.backend.Health(ctx); err != nil {
			return businessError("业务后端当前不可用，请稍后再试")
		}
		return Response{Handled: true, Reply: "业务网关和库存后端运行正常"}
	case "inventory":
		customerCode := binding.CustomerCode
		if isAdmin {
			customerCode = cmd.customerCode
			if customerCode == "" {
				parts := strings.Fields(cmd.keyword)
				if len(parts) > 0 {
					customerCode = parts[0]
					cmd.keyword = strings.Join(parts[1:], " ")
				}
			}
			if customerCode == "" {
				return businessError("管理员查询请使用：查库存 <客户代号> [关键词]")
			}
		} else if cmd.customerCode != "" {
			return businessError("客户群只能查询本群绑定客户的库存")
		}
		inventory, err := s.backend.QueryInventory(ctx, backend.InventoryQuery{
			CustomerCode: customerCode,
			Keyword:      cmd.keyword,
			Limit:        20,
		})
		if err != nil {
			return businessError("库存查询暂时不可用，请稍后再试")
		}
		return Response{Handled: true, Reply: renderInventory(inventory)}
	default:
		return Response{Handled: false}
	}
}

func parseCommand(content string) (command, bool) {
	content = strings.TrimSpace(strings.Trim(content, "，。！？!?"))
	switch content {
	case "业务帮助", "库存帮助", "#业务帮助":
		return command{module: "help", stability: ModuleStable}, true
	case "业务状态", "网关状态", "#业务状态":
		return command{module: "status", stability: ModuleExperimental}, true
	}
	if matches := adminInventoryPattern.FindStringSubmatch(content); len(matches) > 0 {
		return command{
			module:       "inventory",
			stability:    ModuleStable,
			customerCode: strings.TrimSpace(matches[1]),
			keyword:      strings.TrimSpace(matches[2]),
		}, true
	}
	for _, prefix := range []string{"查库存", "库存查询", "库存"} {
		if content == prefix {
			return command{module: "inventory", stability: ModuleStable}, true
		}
		if strings.HasPrefix(content, prefix+" ") {
			return command{module: "inventory", stability: ModuleStable, keyword: strings.TrimSpace(strings.TrimPrefix(content, prefix))}, true
		}
	}
	return command{}, false
}

func stripLeadingMentions(content string) string {
	content = strings.TrimSpace(content)
	for strings.HasPrefix(content, "@") {
		separator := strings.IndexFunc(content, unicode.IsSpace)
		if separator < 0 {
			return content
		}
		_, separatorSize := utf8.DecodeRuneInString(content[separator:])
		content = strings.TrimSpace(content[separator+separatorSize:])
	}
	return content
}

func businessError(message string) Response {
	return Response{Handled: true, Error: message}
}

func renderHelp(admin bool) string {
	lines := []string{
		"业务查询命令",
		"查库存 [关键词]",
		"库存查询 [关键词]",
	}
	if admin {
		lines = append(lines,
			"管理员：查库存 <客户代号> [关键词]",
			"管理员：查 <客户代号> 库存 [关键词]",
			"管理员：业务状态",
		)
	}
	return strings.Join(lines, "\n")
}

func renderInventory(inventory backend.Inventory) string {
	name := strings.TrimSpace(inventory.CustomerName)
	if name == "" {
		name = inventory.CustomerCode
	}
	type inventoryLine struct {
		label string
		qty   float64
	}
	lines := make([]inventoryLine, 0, len(inventory.Items))
	maxLabelWidth := 0
	for _, item := range inventory.Items {
		label := "#" + firstNonEmpty(item.ProductCode, item.ProductName, "未命名货品")
		lines = append(lines, inventoryLine{label: label, qty: item.CartonQty})
		maxLabelWidth = max(maxLabelWidth, textDisplayWidth(label))
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "\n    %s 库存", name)
	for _, line := range lines {
		padding := maxLabelWidth - textDisplayWidth(line.label) + 4
		fmt.Fprintf(&builder, "\n%s%s%s件", line.label, strings.Repeat(" ", padding), formatNumber(line.qty))
	}
	if len(lines) == 0 {
		builder.WriteString("\n未查询到匹配的库存记录")
	}
	fmt.Fprintf(&builder, "\n\n共计%d款，合计%s件", inventory.Summary.Count, formatNumber(inventory.Summary.TotalCartonQty))
	return builder.String()
}

func textDisplayWidth(value string) int {
	width := 0
	for _, char := range value {
		if char <= unicode.MaxASCII {
			width++
		} else {
			width += 2
		}
	}
	return width
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
