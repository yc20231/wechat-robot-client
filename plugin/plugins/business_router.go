package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"wechat-robot-client/interface/plugin"
	"wechat-robot-client/model"
	"wechat-robot-client/pkg/robot"
	"wechat-robot-client/pkg/templates/materialsheet"
	"wechat-robot-client/service"
	"wechat-robot-client/vars"
)

const (
	businessRoutePath          = "/internal/business/route"
	defaultBusinessRouteError  = "业务查询服务暂不可用，请稍后再试"
	defaultBusinessRouteTimout = 5 * time.Second
	defaultBusinessConfigFile  = "/data/skills/.business-gateway.json"
	maxBusinessRouteBody       = 1 << 20
	maxBusinessConfigSize      = 64 << 10
	materialEditorCaptureWidth = 1540
)

type businessRouterConfig struct {
	URL        string `json:"url"`
	Token      string `json:"token"`
	TimeoutSec int    `json:"timeout_sec"`
}

type BusinessRouteRequest struct {
	RobotWxID      string   `json:"robot_wxid"`
	RobotCode      string   `json:"robot_code,omitempty"`
	GroupID        string   `json:"group_id"`
	SenderWxID     string   `json:"sender_wxid"`
	MessageID      int64    `json:"message_id"`
	Content        string   `json:"content"`
	IsAtMe         bool     `json:"is_at_me"`
	MentionedWxIDs []string `json:"mentioned_wxids,omitempty"`
}

type BusinessRouteResponse struct {
	Handled      bool                `json:"handled"`
	Reply        string              `json:"reply,omitempty"`
	Error        string              `json:"error,omitempty"`
	ReplyAtWxIDs []string            `json:"reply_at_wxids,omitempty"`
	Image        *BusinessRouteImage `json:"image,omitempty"`
}

// BusinessRouteImage 是网关下发的配料单网格，与 business-gateway route.MaterialSheetImage 契约一致。
type BusinessRouteImage struct {
	Title              string                        `json:"title"`
	Cells              [][]string                    `json:"cells"`
	Merges             []BusinessRouteImageMerge     `json:"merges,omitempty"`
	Cost               *BusinessRouteCost            `json:"cost,omitempty"`
	SheetName          string                        `json:"sheet_name,omitempty"`
	RowHeaderWidth     float64                       `json:"row_header_width,omitempty"`
	ColumnHeaderHeight float64                       `json:"column_header_height,omitempty"`
	ColumnWidths       []float64                     `json:"column_widths,omitempty"`
	RowHeights         []float64                     `json:"row_heights,omitempty"`
	CellStyles         [][]string                    `json:"cell_styles,omitempty"`
	Styles             map[string]BusinessRouteStyle `json:"styles,omitempty"`
}

type BusinessRouteStyle struct {
	FontFamily string  `json:"font_family,omitempty"`
	FontSize   float64 `json:"font_size,omitempty"`
	Bold       bool    `json:"bold,omitempty"`
	Align      string  `json:"align,omitempty"`
}

type BusinessRouteImageMerge struct {
	StartRow    int `json:"start_row"`
	EndRow      int `json:"end_row"`
	StartColumn int `json:"start_column"`
	EndColumn   int `json:"end_column"`
}

// BusinessRouteCost 与网关 backend.MaterialCostSnapshot 的 JSON 字段对齐。
type BusinessRouteCost struct {
	Status            string                     `json:"status"`
	TotalWeightJin    string                     `json:"total_weight_jin"`
	KnownCost         string                     `json:"known_cost"`
	AverageCostPerJin *string                    `json:"average_cost_per_jin"`
	ProcessingFee     *string                    `json:"processing_fee"`
	MissingMaterials  []string                   `json:"missing_materials"`
	UnsupportedRows   []BusinessRouteUnsupported `json:"unsupported_rows"`
	Regions           []BusinessRouteCostRegion  `json:"regions"`
}

type BusinessRouteCostRegion struct {
	Name           string                 `json:"name"`
	TotalWeightJin string                 `json:"total_weight_jin"`
	TotalCost      *string                `json:"total_cost"`
	Rows           []BusinessRouteCostRow `json:"rows"`
}

type BusinessRouteCostRow struct {
	MaterialName string  `json:"material_name"`
	RawQuantity  string  `json:"raw_quantity"`
	WeightJin    string  `json:"weight_jin"`
	UnitPrice    *string `json:"unit_price"`
	Cost         *string `json:"cost"`
}

type BusinessRouteUnsupported struct {
	MaterialName string `json:"material_name"`
	RawQuantity  string `json:"raw_quantity"`
	Reason       string `json:"reason"`
}

type businessRouteClient interface {
	Route(ctx context.Context, req BusinessRouteRequest) (BusinessRouteResponse, error)
}

type httpBusinessRouteClient struct {
	endpoint string
	token    string
	client   *http.Client
}

func newHTTPBusinessRouteClient(baseURL, token string, timeout time.Duration) (*httpBusinessRouteClient, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("解析 BUSINESS_GATEWAY_URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("BUSINESS_GATEWAY_URL 必须使用 http 或 https")
	}
	if parsed.Host == "" {
		return nil, errors.New("BUSINESS_GATEWAY_URL 缺少主机名")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + businessRoutePath
	if timeout <= 0 {
		timeout = defaultBusinessRouteTimout
	}
	return &httpBusinessRouteClient{
		endpoint: parsed.String(),
		token:    strings.TrimSpace(token),
		client:   &http.Client{Timeout: timeout},
	}, nil
}

func (c *httpBusinessRouteClient) Route(ctx context.Context, routeReq BusinessRouteRequest) (BusinessRouteResponse, error) {
	var result BusinessRouteResponse
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := json.Marshal(routeReq)
	if err != nil {
		return result, fmt.Errorf("编码业务路由请求: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return result, fmt.Errorf("创建业务路由请求: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("X-Internal-Route-Token", c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return result, fmt.Errorf("调用业务网关: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBusinessRouteBody))
	if err != nil {
		return result, fmt.Errorf("读取业务网关响应: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return result, fmt.Errorf("业务网关返回 HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return result, fmt.Errorf("解析业务网关响应: %w", err)
	}
	return result, nil
}

type BusinessRouterPlugin struct {
	client      businessRouteClient
	configErr   error
	captureHTML func(ctx context.Context, html string, width int) ([]byte, error)
}

func NewBusinessRouterPlugin() plugin.MessageHandler {
	config, configured, err := loadBusinessRouterConfig()
	if !configured {
		return &BusinessRouterPlugin{}
	}
	if err != nil {
		log.Printf("[BusinessRouter] 配置无效，将对群消息故障闭合: %v", err)
		return &BusinessRouterPlugin{configErr: err}
	}
	timeout := time.Duration(config.TimeoutSec) * time.Second
	client, err := newHTTPBusinessRouteClient(config.URL, config.Token, timeout)
	if err != nil {
		log.Printf("[BusinessRouter] 配置无效，将对群消息故障闭合: %v", err)
		return &BusinessRouterPlugin{configErr: err}
	}
	return &BusinessRouterPlugin{client: client}
}

func loadBusinessRouterConfig() (businessRouterConfig, bool, error) {
	if baseURL := strings.TrimSpace(os.Getenv("BUSINESS_GATEWAY_URL")); baseURL != "" {
		timeoutSec := int(defaultBusinessRouteTimout / time.Second)
		if raw := strings.TrimSpace(os.Getenv("BUSINESS_GATEWAY_TIMEOUT_SEC")); raw != "" {
			seconds, err := strconv.Atoi(raw)
			if err != nil || seconds <= 0 {
				return businessRouterConfig{}, true, fmt.Errorf("BUSINESS_GATEWAY_TIMEOUT_SEC=%q 无效", raw)
			}
			timeoutSec = seconds
		}
		config := businessRouterConfig{
			URL:        baseURL,
			Token:      strings.TrimSpace(os.Getenv("BUSINESS_GATEWAY_TOKEN")),
			TimeoutSec: timeoutSec,
		}
		if config.Token == "" {
			return businessRouterConfig{}, true, errors.New("BUSINESS_GATEWAY_TOKEN 不能为空")
		}
		return config, true, nil
	}

	configPath := strings.TrimSpace(os.Getenv("BUSINESS_GATEWAY_CONFIG_FILE"))
	if configPath == "" {
		configPath = defaultBusinessConfigFile
	}
	content, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return businessRouterConfig{}, false, nil
	}
	if err != nil {
		return businessRouterConfig{}, true, fmt.Errorf("读取业务网关配置文件: %w", err)
	}
	if len(content) > maxBusinessConfigSize {
		return businessRouterConfig{}, true, errors.New("业务网关配置文件过大")
	}
	var config businessRouterConfig
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return businessRouterConfig{}, true, fmt.Errorf("解析业务网关配置文件: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return businessRouterConfig{}, true, errors.New("业务网关配置文件只能包含一个 JSON 对象")
	}
	config.URL = strings.TrimSpace(config.URL)
	config.Token = strings.TrimSpace(config.Token)
	if config.URL == "" || config.Token == "" {
		return businessRouterConfig{}, true, errors.New("业务网关配置文件必须包含 url 和 token")
	}
	if config.TimeoutSec <= 0 {
		config.TimeoutSec = int(defaultBusinessRouteTimout / time.Second)
	}
	return config, true, nil
}

func (p *BusinessRouterPlugin) GetName() string {
	return "BusinessRouter"
}

func (p *BusinessRouterPlugin) GetLabels() []string {
	return []string{"text", "chat", "business"}
}

func (p *BusinessRouterPlugin) Match(ctx *plugin.MessageContext) bool {
	return (p.client != nil || p.configErr != nil) && ctx != nil && ctx.Message != nil && ctx.Message.IsChatRoom && ctx.Message.IsAtMe
}

func (p *BusinessRouterPlugin) PreAction(ctx *plugin.MessageContext) bool {
	return p.Match(ctx) && NewChatRoomCommonPlugin().PreAction(ctx)
}

func (p *BusinessRouterPlugin) PostAction(ctx *plugin.MessageContext) {}

func (p *BusinessRouterPlugin) Run(ctx *plugin.MessageContext) {
	if !p.PreAction(ctx) {
		return
	}
	if p.configErr != nil {
		p.replyAndStop(ctx, defaultBusinessRouteError)
		return
	}
	routeContext := ctx.Context
	if routeContext == nil {
		routeContext = context.Background()
	}
	response, err := p.client.Route(routeContext, BusinessRouteRequest{
		RobotWxID:      vars.RobotRuntime.WxID,
		RobotCode:      vars.RobotRuntime.RobotCode,
		GroupID:        ctx.Message.FromWxID,
		SenderWxID:     ctx.Message.SenderWxID,
		MessageID:      ctx.Message.MsgId,
		Content:        ctx.MessageContent,
		IsAtMe:         ctx.Message.IsAtMe,
		MentionedWxIDs: extractMentionedWxIDs(ctx.Message),
	})
	if err != nil {
		log.Printf("[BusinessRouter] 路由失败 msg_id=%d: %v", ctx.Message.MsgId, err)
		p.replyAndStop(ctx, defaultBusinessRouteError)
		return
	}
	if !response.Handled {
		return
	}
	reply := response.Reply
	if strings.TrimSpace(response.Error) != "" {
		reply = strings.TrimSpace(response.Error)
	}
	if response.Image != nil && len(response.Image.Cells) > 0 && strings.TrimSpace(response.Error) == "" {
		p.sendImageThenReply(ctx, response.Image, reply, response.ReplyAtWxIDs...)
		return
	}
	p.replyAndStop(ctx, reply, response.ReplyAtWxIDs...)
}

// sendImageThenReply 渲染编辑器样式截图并发送；成功则不再发文字，失败才降级为文字。
func (p *BusinessRouterPlugin) sendImageThenReply(ctx *plugin.MessageContext, image *BusinessRouteImage, reply string, extraAtWxIDs ...string) {
	merges := make([]materialsheet.Merge, 0, len(image.Merges))
	for _, merge := range image.Merges {
		merges = append(merges, materialsheet.Merge{
			StartRow: merge.StartRow, EndRow: merge.EndRow,
			StartColumn: merge.StartColumn, EndColumn: merge.EndColumn,
		})
	}
	htmlContent := materialsheet.Render(materialsheet.Document{
		Title:  image.Title,
		Cells:  image.Cells,
		Merges: merges,
		Cost:   costPanelFromRoute(image.Cost),
		Layout: sheetLayoutFromRoute(image),
	})
	routeContext := ctx.Context
	if routeContext == nil {
		routeContext = context.Background()
	}
	pngBytes, err := p.capture(routeContext, htmlContent, materialEditorCaptureWidth)
	if err != nil {
		log.Printf("[BusinessRouter] 渲染业务图片失败 msg_id=%d: %v", ctx.Message.MsgId, err)
		p.replyAndStop(ctx, reply, extraAtWxIDs...)
		return
	}
	if _, err := ctx.MessageService.MsgUploadImg(ctx.Message.FromWxID, bytes.NewReader(pngBytes)); err != nil {
		log.Printf("[BusinessRouter] 发送业务图片失败 msg_id=%d: %v", ctx.Message.MsgId, err)
		p.replyAndStop(ctx, reply, extraAtWxIDs...)
		return
	}
	ctx.Handled = true
}

func (p *BusinessRouterPlugin) capture(ctx context.Context, html string, width int) ([]byte, error) {
	if p.captureHTML != nil {
		return p.captureHTML(ctx, html, width)
	}
	return service.CaptureHTMLScreenshotWidth(ctx, html, width)
}

func sheetLayoutFromRoute(image *BusinessRouteImage) *materialsheet.SheetLayout {
	if image == nil {
		return nil
	}
	if len(image.ColumnWidths) == 0 && len(image.RowHeights) == 0 && len(image.Styles) == 0 {
		return nil
	}
	layout := &materialsheet.SheetLayout{
		SheetName:          image.SheetName,
		RowHeaderWidth:     image.RowHeaderWidth,
		ColumnHeaderHeight: image.ColumnHeaderHeight,
		ColumnWidths:       image.ColumnWidths,
		RowHeights:         image.RowHeights,
		CellStyles:         image.CellStyles,
	}
	if len(image.Styles) > 0 {
		layout.Styles = make(map[string]materialsheet.CellStyle, len(image.Styles))
		for id, style := range image.Styles {
			layout.Styles[id] = materialsheet.CellStyle{
				FontFamily: style.FontFamily,
				FontSize:   style.FontSize,
				Bold:       style.Bold,
				Align:      style.Align,
			}
		}
	}
	return layout
}

func costPanelFromRoute(cost *BusinessRouteCost) *materialsheet.CostPanel {
	if cost == nil {
		return nil
	}
	panel := &materialsheet.CostPanel{
		Status:            cost.Status,
		TotalWeightJin:    cost.TotalWeightJin,
		KnownCost:         cost.KnownCost,
		AverageCostPerJin: cost.AverageCostPerJin,
		ProcessingFee:     cost.ProcessingFee,
		MissingMaterials:  cost.MissingMaterials,
	}
	for _, row := range cost.UnsupportedRows {
		panel.UnsupportedRows = append(panel.UnsupportedRows, materialsheet.UnsupportedRow{
			MaterialName: row.MaterialName, RawQuantity: row.RawQuantity, Reason: row.Reason,
		})
	}
	for _, region := range cost.Regions {
		item := materialsheet.CostRegion{
			Name: region.Name, TotalWeightJin: region.TotalWeightJin, TotalCost: region.TotalCost,
		}
		for _, row := range region.Rows {
			item.Rows = append(item.Rows, materialsheet.CostRow{
				MaterialName: row.MaterialName, RawQuantity: row.RawQuantity, WeightJin: row.WeightJin,
				UnitPrice: row.UnitPrice, Cost: row.Cost,
			})
		}
		panel.Regions = append(panel.Regions, item)
	}
	return panel
}

func extractMentionedWxIDs(message *model.Message) []string {
	if message == nil || strings.TrimSpace(message.MessageSource) == "" {
		return nil
	}
	var source robot.MessageSource
	if err := vars.RobotRuntime.XmlDecoder(message.MessageSource, &source); err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, raw := range strings.Split(source.AtUserList, ",") {
		wxID := strings.TrimSpace(raw)
		if wxID == "" {
			continue
		}
		if _, ok := seen[wxID]; ok {
			continue
		}
		seen[wxID] = struct{}{}
		result = append(result, wxID)
	}
	return result
}

func (p *BusinessRouterPlugin) replyAndStop(ctx *plugin.MessageContext, reply string, extraAtWxIDs ...string) {
	ctx.Handled = true
	if strings.TrimSpace(reply) == "" {
		return
	}
	atWxIDs := []string{ctx.Message.SenderWxID}
	seen := map[string]struct{}{ctx.Message.SenderWxID: {}}
	for _, wxID := range extraAtWxIDs {
		wxID = strings.TrimSpace(wxID)
		if wxID == "" || wxID == vars.RobotRuntime.WxID {
			continue
		}
		if _, ok := seen[wxID]; ok {
			continue
		}
		seen[wxID] = struct{}{}
		atWxIDs = append(atWxIDs, wxID)
	}
	if err := ctx.MessageService.SendTextMessage(ctx.Message.FromWxID, reply, atWxIDs...); err != nil {
		log.Printf("[BusinessRouter] 发送业务回复失败 msg_id=%d: %v", ctx.Message.MsgId, err)
	}
}
