package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"business-gateway/internal/admin"
	"business-gateway/internal/backend"
	"business-gateway/internal/config"
	"business-gateway/internal/dedup"
	"business-gateway/internal/group"
	"business-gateway/internal/route"
)

type fakeBackend struct{}

func (fakeBackend) QueryInventory(context.Context, backend.InventoryQuery) (backend.Inventory, error) {
	return backend.Inventory{CustomerCode: "270"}, nil
}
func (fakeBackend) Health(context.Context) error { return nil }
func (fakeBackend) ResolveCustomer(context.Context, string) (backend.Customer, error) {
	return backend.Customer{Exists: true, Code: "270", Name: "测试客户"}, nil
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	groups, err := group.NewFileStore(filepath.Join(t.TempDir(), "groups.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := groups.Upsert(group.Binding{GroupID: "customer@chatroom", Type: group.TypeCustomer, CustomerCode: "270", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	cache := dedup.NewMemoryCache(time.Hour)
	cfg := config.Config{InternalRouteToken: "route-secret", WebhookToken: "webhook-secret", AdminToken: "admin-secret"}
	admins, err := admin.NewFileStore(filepath.Join(t.TempDir(), "admins.json"), map[string]struct{}{"owner-wxid": {}})
	if err != nil {
		t.Fatal(err)
	}
	router := route.NewService(groups, fakeBackend{}, cache, admins, nil, true, 5*time.Minute)
	return NewHandler(cfg, groups, router, cache)
}

func TestInternalRouteAuthenticationAndResponse(t *testing.T) {
	handler := newTestHandler(t)
	body := []byte(`{"robot_wxid":"robot","group_id":"customer@chatroom","sender_wxid":"member","message_id":1,"content":"查库存","is_at_me":true}`)

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/internal/business/route", bytes.NewReader(body)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/business/route", bytes.NewReader(body))
	req.Header.Set("X-Internal-Route-Token", "route-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	var result route.Response
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.Handled || result.Error != "" {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestWebhookAcceptsRealisticExtraFieldsAndDeduplicates(t *testing.T) {
	handler := newTestHandler(t)
	body := []byte(`{
		"Appid":"app-1",
		"Wxid":"robot",
		"SyncKey":"ignored-extra-field",
		"AddMsgs":[
			{"FromUserName":{"string":"member"},"ToUserName":{"string":"robot"},"Content":{"string":"查库存"},"MsgType":1,"NewMsgId":99,"Extra":"ignored"},
			{"FromUserName":{"string":"member"},"ToUserName":{"string":"robot"},"Content":{"string":"查库存"},"MsgType":1,"NewMsgId":99},
			{"FromUserName":{"string":"robot"},"ToUserName":{"string":"group"},"Content":{"string":"reply"},"MsgType":1,"NewMsgId":100}
		]
	}`)
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/webhook/wechat", bytes.NewReader(body)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized webhook status = %d", unauthorized.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook/wechat?robot_wxid=robot", bytes.NewReader(body))
	req.Header.Set("X-Bot-Webhook-Token", "webhook-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	var result map[string]int
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["accepted"] != 1 || result["duplicates"] != 1 || result["own_messages"] != 1 {
		t.Fatalf("unexpected counters: %+v", result)
	}
}

func TestAdminGroupEndpointRequiresTokenAndPersistsBinding(t *testing.T) {
	handler := newTestHandler(t)
	body := []byte(`{"group_id":"new@chatroom","group_name":"新客户群","type":"customer","customer_code":"300","enabled":true}`)

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/admin/groups", bytes.NewReader(body)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized admin status = %d", unauthorized.Code)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/admin/groups", bytes.NewReader(body))
	createReq.Header.Set("X-Admin-Token", "admin-secret")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, createReq)
	if created.Code != http.StatusOK {
		t.Fatalf("create status = %d, body=%s", created.Code, created.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/admin/groups", nil)
	listReq.Header.Set("X-Admin-Token", "admin-secret")
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, listReq)
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte(`"group_id":"new@chatroom"`)) {
		t.Fatalf("list status = %d, body=%s", listed.Code, listed.Body.String())
	}
}
