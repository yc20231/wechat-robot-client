package route

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"business-gateway/internal/backend"
)

// 配料成本查询：仅限机器人管理员使用（用户要求），结果带一张配料单图片，
// 图片网格由机器人端渲染；本文件只负责指令解析、权限、候选选择和文本排版。

var (
	// costSelectionPattern 匹配多候选后的二次选择，如 “查成本 #2”。
	costSelectionPattern = regexp.MustCompile(`^(?:查成本|查配料成本|配料成本)\s*#?([0-9]+)$`)
	// costNaturalPattern 匹配自然说法，如 “查询网的18厚白配料成本”“网的18厚白配料成本是多少”。
	costNaturalPattern = regexp.MustCompile(`^(?:查[询看]?\s*)?(.{1,60}?)(?:的)?配料成本(?:是|为)?多少(?:钱)?[吗么]?$`)
	costQueryPrefixes  = []string{"查配料成本", "查询成本", "查成本"}
)

// MaterialSheetMerge 与 backend.MaterialSheetMerge 的 JSON 契约保持一致。
type MaterialSheetMerge struct {
	StartRow    int `json:"start_row"`
	EndRow      int `json:"end_row"`
	StartColumn int `json:"start_column"`
	EndColumn   int `json:"end_column"`
}

// MaterialSheetImage 随回复下发，机器人端据此渲染配料单图片。
type MaterialSheetImage struct {
	Title  string               `json:"title"`
	Cells  [][]string           `json:"cells"`
	Merges []MaterialSheetMerge `json:"merges,omitempty"`
}

// parseCostCommand 解析成本查询指令；与库存指令互不重叠（库存先解析）。
func parseCostCommand(content string) (command, bool) {
	if matches := costSelectionPattern.FindStringSubmatch(content); matches != nil {
		return command{module: "cost", selection: matches[1]}, true
	}
	for _, prefix := range costQueryPrefixes {
		if content == prefix {
			return command{module: "cost"}, true
		}
		if strings.HasPrefix(content, prefix+" ") {
			return command{module: "cost", keyword: strings.TrimSpace(strings.TrimPrefix(content, prefix))}, true
		}
	}
	if matches := costNaturalPattern.FindStringSubmatch(content); matches != nil {
		if keyword := strings.TrimSpace(matches[1]); keyword != "" {
			return command{module: "cost", keyword: keyword}, true
		}
	}
	return command{}, false
}

// costSelection 记录一个群成员最近一次多候选查询，供“查成本 #序号”选择。
type costSelection struct {
	keyword   string
	fileIDs   []int64
	expiresAt time.Time
}

const costSelectionTTL = 10 * time.Minute

func (s *Service) handleCost(ctx context.Context, req Request, cmd command) Response {
	if s.admins == nil || !s.admins.IsAdmin(req.SenderWxID) {
		return businessError("配料成本查询仅限机器人管理员使用")
	}
	if cmd.selection != "" {
		return s.handleCostSelection(ctx, req, cmd.selection)
	}
	keyword := normalizeBotKeyword(strings.TrimSpace(cmd.keyword))
	if keyword == "" {
		return Response{Handled: true, Reply: "配料成本查询用法：查成本 <客户+货号>，如：查成本 网的18厚白"}
	}
	cost, err := s.backend.QueryMaterialCost(ctx, keyword, 0)
	if err != nil {
		var backendErr *backend.BackendError
		if errors.As(err, &backendErr) && strings.TrimSpace(backendErr.Message) != "" {
			return businessError(backendErr.Message)
		}
		return businessError("配料成本查询暂时不可用，请稍后再试")
	}
	if !cost.Resolved {
		return s.replyCostMatches(req, keyword, cost)
	}
	// 客户对不上时不直接出结果：如“网的18厚白”匹配到 026 的《18厚白》，
	// 转为候选列表让管理员确认，避免把别的客户的单子发出去。
	if desiredCustomer, _ := splitBotKeyword(keyword); desiredCustomer != "" &&
		cost.CustomerCode != "" && cost.CustomerCode != desiredCustomer {
		return s.replyCostCustomerMismatch(req, keyword, desiredCustomer, cost)
	}
	return costResolvedResponse(cost)
}

// handleCostSelection 处理“查成本 #序号”，从最近候选中取出单据再查一次。
func (s *Service) handleCostSelection(ctx context.Context, req Request, selection string) Response {
	index, err := strconv.Atoi(selection)
	if err != nil || index < 1 {
		return businessError("序号格式不正确，请回复：查成本 #序号")
	}
	selections := s.takeCostSelections(req.GroupID, req.SenderWxID)
	if selections == nil || index > len(selections.fileIDs) {
		return businessError("没有可用的候选列表或已过期，请重新发送查成本指令")
	}
	fileID := selections.fileIDs[index-1]
	cost, err := s.backend.QueryMaterialCost(ctx, selections.keyword, fileID)
	if err != nil {
		var backendErr *backend.BackendError
		if errors.As(err, &backendErr) && strings.TrimSpace(backendErr.Message) != "" {
			return businessError(backendErr.Message)
		}
		return businessError("配料成本查询暂时不可用，请稍后再试")
	}
	if !cost.Resolved {
		return businessError("该候选已失效，请重新发送查成本指令")
	}
	return costResolvedResponse(cost)
}

func (s *Service) replyCostMatches(req Request, keyword string, cost backend.MaterialCost) Response {
	if len(cost.Matches) == 0 {
		return Response{Handled: true, Reply: "没找到与「" + keyword + "」匹配的配料单，请确认名称后重试，如：查成本 网的18厚白"}
	}
	fileIDs := make([]int64, 0, len(cost.Matches))
	lines := make([]string, 0, len(cost.Matches))
	for _, match := range cost.Matches {
		fileIDs = append(fileIDs, match.FileID)
		line := fmt.Sprintf("#%d %s", match.Index, match.Name)
		parts := make([]string, 0, 2)
		if match.CustomerCode != "" {
			parts = append(parts, "客户："+match.CustomerCode)
		}
		if match.ArticleNumber != "" {
			parts = append(parts, "货号："+match.ArticleNumber)
		}
		if len(parts) > 0 {
			line += "（" + strings.Join(parts, " ") + "）"
		}
		lines = append(lines, line)
	}
	s.storeCostSelections(req.GroupID, req.SenderWxID, costSelection{
		keyword:   keyword,
		fileIDs:   fileIDs,
		expiresAt: time.Now().Add(costSelectionTTL),
	})
	reply := "找到多张匹配的配料单，请回复“查成本 #序号”查看：\n" + strings.Join(lines, "\n")
	return Response{Handled: true, Reply: reply}
}

func costResolvedResponse(cost backend.MaterialCost) Response {
	reply := renderCostReply(cost)
	response := Response{Handled: true, Reply: reply}
	if cost.Sheet != nil && len(cost.Sheet.Cells) > 0 {
		image := &MaterialSheetImage{Title: costSheetTitle(cost), Cells: cost.Sheet.Cells}
		for _, merge := range cost.Sheet.Merges {
			image.Merges = append(image.Merges, MaterialSheetMerge{
				StartRow: merge.StartRow, EndRow: merge.EndRow,
				StartColumn: merge.StartColumn, EndColumn: merge.EndColumn,
			})
		}
		response.Image = image
	}
	return response
}

func costSheetTitle(cost backend.MaterialCost) string {
	if cost.File != nil {
		name := strings.TrimSpace(cost.File.Name)
		if index := strings.LastIndex(name, "."); index > 0 {
			name = name[:index]
		}
		if name != "" {
			return name
		}
	}
	if cost.Query != "" {
		return cost.Query
	}
	return "配料生产安排单"
}

func renderCostReply(cost backend.MaterialCost) string {
	title := costSheetTitle(cost)
	snapshot := cost.Cost
	if snapshot == nil {
		return "配料单「" + title + "」缺少成本数据"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "【%s】配料成本", title)
	if tons := costPerTonText(snapshot.AverageCostPerJin); tons != "" {
		fmt.Fprintf(&builder, "\n成本：%s元/吨", tons)
	}
	totalCost := "—"
	if snapshot.TotalCost != nil {
		totalCost = *snapshot.TotalCost
	}
	fmt.Fprintf(&builder, "\n共计：%s斤 = %s元", snapshot.TotalWeightJin, totalCost)
	if snapshot.ProductionCost != nil && *snapshot.ProductionCost != "" {
		fmt.Fprintf(&builder, "\n投产：%s斤，成本 %s元", orDash(snapshot.ProductionWeightJin), *snapshot.ProductionCost)
	}
	fmt.Fprintf(&builder, "\n加工费：%s", orDash(snapshot.ProcessingFee))
	for _, region := range snapshot.Regions {
		fmt.Fprintf(&builder, "\n%s区域 共%s斤", region.Name, region.TotalWeightJin)
		for _, row := range region.Rows {
			line := fmt.Sprintf("· %s %s → %s斤", row.MaterialName, row.RawQuantity, row.WeightJin)
			if row.Cost != nil && *row.Cost != "" {
				line += " = ¥" + *row.Cost
			}
			builder.WriteString("\n")
			builder.WriteString(line)
		}
	}
	if len(snapshot.MissingMaterials) > 0 {
		fmt.Fprintf(&builder, "\n缺价格：%s", strings.Join(snapshot.MissingMaterials, "、"))
	}
	if len(snapshot.UnsupportedRows) > 0 {
		names := make([]string, 0, len(snapshot.UnsupportedRows))
		for _, row := range snapshot.UnsupportedRows {
			names = append(names, row.MaterialName)
		}
		fmt.Fprintf(&builder, "\n无法识别：%s", strings.Join(names, "、"))
	}
	if snapshot.Status != "complete" {
		builder.WriteString("\n（部分原料缺少价格，成本可能不完整）")
	}
	return builder.String()
}

// costPerTonText 把元/斤换算成元/吨，与编辑器面板口径一致。
func costPerTonText(averageCostPerJin *string) string {
	if averageCostPerJin == nil || *averageCostPerJin == "" {
		return ""
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(*averageCostPerJin), 64)
	if err != nil || value <= 0 {
		return ""
	}
	tons := value * 2000
	return strconv.FormatFloat(tons, 'f', -1, 64)
}

func orDash(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "—"
	}
	return strings.TrimSpace(*value)
}

// splitBotKeyword 把“网的18厚白”拆成客户代号“网”和货号“18厚白”；没有“的”时整体视为货号。
func splitBotKeyword(keyword string) (customer, article string) {
	index := strings.Index(keyword, "的")
	if index < 0 {
		return "", keyword
	}
	return strings.TrimSpace(keyword[:index]), strings.TrimSpace(keyword[index+len("的"):])
}

// normalizeBotKeyword 去掉关键词里的全部空白（含全角空格）：中文单据名不带空格，
// 而手机输入法常在中文和数字间插空格（如“网的 18 厚白”）。
func normalizeBotKeyword(keyword string) string {
	var builder strings.Builder
	for _, char := range keyword {
		if unicode.IsSpace(char) {
			continue
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

// replyCostCustomerMismatch 客户代号不符时列出相近结果，供管理员手动确认。
func (s *Service) replyCostCustomerMismatch(req Request, keyword, desiredCustomer string, cost backend.MaterialCost) Response {
	fileID := int64(0)
	name := keyword
	if cost.File != nil {
		fileID = cost.File.ID
		name = cost.File.Name
	}
	hint := ""
	if fileID > 0 {
		s.storeCostSelections(req.GroupID, req.SenderWxID, costSelection{
			keyword:   keyword,
			fileIDs:   []int64{fileID},
			expiresAt: time.Now().Add(costSelectionTTL),
		})
		hint = "\n如需查看这张单子，回复“查成本 #1”"
	}
	return Response{Handled: true, Reply: fmt.Sprintf(
		"没找到客户「%s」的配料单，最接近的是：%s（客户代号：%s）%s",
		desiredCustomer, name, cost.CustomerCode, hint,
	)}
}

// ---- 候选列表存取（按群+发送者隔离，带过期清理） ----

func selectionKey(groupID, senderWxID string) string {
	return groupID + "\x00" + senderWxID
}

func (s *Service) storeCostSelections(groupID, senderWxID string, selection costSelection) {
	s.costMu.Lock()
	defer s.costMu.Unlock()
	if s.costSelections == nil {
		s.costSelections = map[string]costSelection{}
	}
	s.costSelections[selectionKey(groupID, senderWxID)] = selection
	s.expireCostSelectionsLocked(time.Now())
}

func (s *Service) takeCostSelections(groupID, senderWxID string) *costSelection {
	s.costMu.Lock()
	defer s.costMu.Unlock()
	key := selectionKey(groupID, senderWxID)
	selection, ok := s.costSelections[key]
	if !ok || time.Now().After(selection.expiresAt) {
		delete(s.costSelections, key)
		return nil
	}
	// 只读复用：同一列表允许多次按序号选择，直到过期。
	return &selection
}

func (s *Service) expireCostSelectionsLocked(now time.Time) {
	for key, selection := range s.costSelections {
		if now.After(selection.expiresAt) {
			delete(s.costSelections, key)
		}
	}
}
