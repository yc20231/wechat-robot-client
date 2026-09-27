// Package notices polls the ERP outbox. It never retries a WeChat send: a lost
// response is an unknown result, not permission to send again.
package notices

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
	"strings"
	"time"

	"business-gateway/internal/group"
)

type Config struct {
	BackendURL, BackendToken, RobotURL, RobotToken string
	AllowedGroups                                  []string
}
type Worker struct {
	cfg    Config
	groups group.Store
	http   *http.Client
}
type task struct {
	ID         int64  `json:"id"`
	LeaseToken string `json:"lease_token"`
	SentCount  int    `json:"sent_count"`
	Payload    struct {
		CustomerCode string   `json:"customer_code"`
		GroupID      string   `json:"group_id"`
		Messages     []string `json:"messages"`
	} `json:"payload"`
}

func New(cfg Config, groups group.Store) (*Worker, error) {
	for _, base := range []string{cfg.BackendURL, cfg.RobotURL} {
		u, err := url.Parse(base)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("通知服务 URL 无效")
		}
	}
	if len(cfg.BackendToken) < 32 || len(cfg.RobotToken) < 32 {
		return nil, errors.New("ERP 和机器人认证凭据均须至少 32 字符，可显式复用现有凭据")
	}
	if cfg.BackendToken == cfg.RobotToken {
		return nil, errors.New("ERP 和机器人必须使用不同的通知 Token")
	}
	if len(cfg.AllowedGroups) == 0 {
		return nil, errors.New("必须明确配置允许发送的群 ID；首次联调仅填写测试群")
	}
	for _, id := range cfg.AllowedGroups {
		if !strings.HasSuffix(id, "@chatroom") {
			return nil, errors.New("通知群白名单无效")
		}
	}
	return &Worker{cfg: cfg, groups: groups, http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := w.Poll(ctx); err != nil && ctx.Err() == nil {
			log.Printf("生产通知轮询失败（不会重发）: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (w *Worker) request(ctx context.Context, robot bool, method, path string, body, target any) error {
	base, token := w.cfg.BackendURL, w.cfg.BackendToken
	header := "X-Bot-Token"
	if robot {
		base, token, header = w.cfg.RobotURL, w.cfg.RobotToken, "X-Production-Notice-Token"
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(header, token)
	req.Header.Set("User-Agent", "bot-mcp/production-notices-1.0")
	resp, err := w.http.Do(req)
	if err != nil {
		return errors.New("通知网络请求失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("通知接口 HTTP %d", resp.StatusCode)
	}
	var envelope struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope); err != nil {
		return errors.New("通知响应无法解析")
	}
	if envelope.Code == nil || *envelope.Code != 0 {
		return errors.New("通知接口拒绝请求")
	}
	if target != nil {
		if err = json.Unmarshal(envelope.Data, target); err != nil {
			return errors.New("通知响应数据无效")
		}
	}
	return nil
}
func (w *Worker) ready(ctx context.Context) bool {
	var result struct {
		Ready bool `json:"ready"`
	}
	return w.request(ctx, true, http.MethodGet, "/api/v1/robot/production-notice/health", nil, &result) == nil && result.Ready
}
func (w *Worker) validBinding(t task) bool {
	if !w.allowed(t.Payload.GroupID) {
		return false
	}
	count := 0
	for _, b := range w.groups.List() {
		if b.Enabled && b.Type == group.TypeCustomer && b.CustomerCode == t.Payload.CustomerCode {
			count++
			if b.GroupID != t.Payload.GroupID {
				return false
			}
		}
	}
	return count == 1 && strings.HasSuffix(t.Payload.GroupID, "@chatroom")
}
func (w *Worker) allowed(id string) bool {
	for _, g := range w.cfg.AllowedGroups {
		if g == id {
			return true
		}
	}
	return false
}
func (w *Worker) Poll(ctx context.Context) error {
	ready := w.ready(ctx)
	// Only customer bindings are exposed to the ERP, never administrator groups.
	bindings := []group.Binding{}
	for _, b := range w.groups.List() {
		if b.Type == group.TypeCustomer && strings.HasSuffix(b.GroupID, "@chatroom") {
			if !w.allowed(b.GroupID) {
				b.Enabled = false
			}
			bindings = append(bindings, b)
		}
	}
	if err := w.request(ctx, false, http.MethodPost, "/api/bot/production-notices/heartbeat", map[string]any{"bindings": bindings, "ready": ready}, nil); err != nil {
		return err
	}
	if !ready {
		return nil
	}
	var t *task
	if err := w.request(ctx, false, http.MethodPost, "/api/bot/production-notices/claim", struct{}{}, &t); err != nil {
		return err
	}
	if t == nil {
		return nil
	}
	if len(t.Payload.Messages) != 2 || t.SentCount < 0 || t.SentCount > 1 || t.LeaseToken == "" {
		return errors.New("任务结构无效；等待人工核对")
	}
	for i := t.SentCount; i < 2; i++ {
		step := map[string]any{"lease_token": t.LeaseToken, "index": i}
		base := fmt.Sprintf("/api/bot/production-notices/%d", t.ID)
		if !w.validBinding(*t) || !w.ready(ctx) {
			step["outcome"] = "blocked"
			return w.request(ctx, false, http.MethodPost, base+"/result", step, nil)
		}
		if err := w.request(ctx, false, http.MethodPost, base+"/begin", step, nil); err != nil {
			// Even a lost begin response cannot cause a send. A blocked result is only
			// accepted if begin truly did not commit; otherwise the lease becomes unknown.
			step["outcome"] = "blocked"
			_ = w.request(ctx, false, http.MethodPost, base+"/result", step, nil)
			return err
		}
		var receipt struct {
			Accepted bool `json:"accepted"`
		}
		err := w.request(ctx, true, http.MethodPost, "/api/v1/robot/production-notice/send", map[string]any{"to_wxid": t.Payload.GroupID, "content": t.Payload.Messages[i]}, &receipt)
		if err != nil || !receipt.Accepted {
			step["outcome"] = "unknown"
			reportErr := w.request(ctx, false, http.MethodPost, base+"/result", step, nil)
			return errors.Join(errors.New("发送结果不确定，任务停止，禁止自动重发"), err, reportErr)
		}
		step["outcome"] = "sent"
		if err = w.request(ctx, false, http.MethodPost, base+"/result", step, nil); err != nil {
			return err
		}
		if i == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}
	return nil
}
