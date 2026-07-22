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
	"wechat-robot-client/vars"
)

const (
	businessRoutePath          = "/internal/business/route"
	defaultBusinessRouteError  = "业务查询服务暂不可用，请稍后再试"
	defaultBusinessRouteTimout = 5 * time.Second
	defaultBusinessConfigFile  = "/data/skills/.business-gateway.json"
	maxBusinessRouteBody       = 1 << 20
	maxBusinessConfigSize      = 64 << 10
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
	Handled      bool     `json:"handled"`
	Reply        string   `json:"reply,omitempty"`
	Error        string   `json:"error,omitempty"`
	ReplyAtWxIDs []string `json:"reply_at_wxids,omitempty"`
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
	client    businessRouteClient
	configErr error
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
	reply := strings.TrimSpace(response.Reply)
	if strings.TrimSpace(response.Error) != "" {
		reply = strings.TrimSpace(response.Error)
	}
	p.replyAndStop(ctx, reply, response.ReplyAtWxIDs...)
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
