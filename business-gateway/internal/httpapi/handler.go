package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"business-gateway/internal/config"
	"business-gateway/internal/dedup"
	"business-gateway/internal/group"
	"business-gateway/internal/route"
)

const maxRequestBody = 8 << 20

type handler struct {
	cfg    config.Config
	groups group.Store
	router *route.Service
	dedup  dedup.Cache
}

func NewHandler(cfg config.Config, groups group.Store, router *route.Service, cache dedup.Cache) http.Handler {
	h := &handler{cfg: cfg, groups: groups, router: router, dedup: cache}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("POST /internal/business/route", h.businessRoute)
	mux.HandleFunc("POST /webhook/wechat", h.webhook)
	mux.HandleFunc("GET /admin/groups", h.listGroups)
	mux.HandleFunc("POST /admin/groups", h.upsertGroup)
	mux.HandleFunc("PUT /admin/groups/{group_id}", h.upsertGroup)
	mux.HandleFunc("DELETE /admin/groups/{group_id}", h.deleteGroup)
	return requestLogger(mux)
}

func (h *handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *handler) businessRoute(w http.ResponseWriter, r *http.Request) {
	if !validToken(r.Header.Get("X-Internal-Route-Token"), h.cfg.InternalRouteToken) {
		writeError(w, http.StatusUnauthorized, "内部路由 token 无效")
		return
	}
	var req route.Request
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	response := h.router.Route(r.Context(), req)
	writeJSON(w, http.StatusOK, response)
}

type webhookMessage struct {
	FromUserName stringOrObject `json:"FromUserName"`
	ToUserName   stringOrObject `json:"ToUserName"`
	Content      stringOrObject `json:"Content"`
	MsgType      int            `json:"MsgType"`
	NewMsgID     int64          `json:"NewMsgId"`
}

type webhookPayload struct {
	AppID   string           `json:"Appid"`
	WxID    string           `json:"Wxid"`
	AddMsgs []webhookMessage `json:"AddMsgs"`
}

type stringOrObject struct {
	Value string
}

func (s *stringOrObject) UnmarshalJSON(data []byte) error {
	var direct string
	if json.Unmarshal(data, &direct) == nil {
		s.Value = direct
		return nil
	}
	var wrapped struct {
		String *string `json:"string"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return err
	}
	if wrapped.String != nil {
		s.Value = *wrapped.String
	}
	return nil
}

func (h *handler) webhook(w http.ResponseWriter, r *http.Request) {
	if !validToken(r.Header.Get("X-Bot-Webhook-Token"), h.cfg.WebhookToken) {
		writeError(w, http.StatusUnauthorized, "Webhook token 无效")
		return
	}
	var payload webhookPayload
	if err := decodeJSONLenient(w, r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	robotWxID := strings.TrimSpace(r.URL.Query().Get("robot_wxid"))
	if robotWxID == "" {
		robotWxID = payload.WxID
	}
	accepted, duplicates, ownMessages := 0, 0, 0
	for _, message := range payload.AddMsgs {
		if robotWxID != "" && message.FromUserName.Value == robotWxID {
			ownMessages++
			continue
		}
		key := fmt.Sprintf("webhook:%s:%d", payload.AppID, message.NewMsgID)
		if message.NewMsgID != 0 && h.dedup.Seen(key) {
			duplicates++
			continue
		}
		accepted++
		log.Printf("[WebhookAudit] appid=%s msg_id=%d from=%s to=%s type=%d", payload.AppID, message.NewMsgID, message.FromUserName.Value, message.ToUserName.Value, message.MsgType)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accepted":     accepted,
		"duplicates":   duplicates,
		"own_messages": ownMessages,
	})
}

func (h *handler) listGroups(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": h.groups.List()})
}

func (h *handler) upsertGroup(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	var binding group.Binding
	if err := decodeJSON(w, r, &binding); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if pathID := r.PathValue("group_id"); pathID != "" {
		decoded, err := url.PathUnescape(pathID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "group_id 无效")
			return
		}
		if binding.GroupID != "" && binding.GroupID != decoded {
			writeError(w, http.StatusBadRequest, "路径和请求体的 group_id 不一致")
			return
		}
		binding.GroupID = decoded
	}
	if err := h.groups.Upsert(binding); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, binding)
}

func (h *handler) deleteGroup(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	groupID, err := url.PathUnescape(r.PathValue("group_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "group_id 无效")
		return
	}
	if err := h.groups.Delete(groupID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": groupID})
}

func (h *handler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !validToken(r.Header.Get("X-Admin-Token"), h.cfg.AdminToken) {
		writeError(w, http.StatusUnauthorized, "管理 token 无效")
		return false
	}
	return true
}

func validToken(provided, expected string) bool {
	provided = strings.TrimSpace(provided)
	if provided == "" || expected == "" || len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	return decodeJSONWithMode(w, r, target, true)
}

func decodeJSONLenient(w http.ResponseWriter, r *http.Request, target any) error {
	return decodeJSONWithMode(w, r, target, false)
}

func decodeJSONWithMode(w http.ResponseWriter, r *http.Request, target any, strict bool) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("JSON 请求无效: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("JSON 请求只能包含一个对象")
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSONStatus(w, status, map[string]any{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	writeJSONStatus(w, status, value)
}

func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("写入 JSON 响应失败: %v", err)
	}
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[HTTP] method=%s path=%s remote=%s", r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
