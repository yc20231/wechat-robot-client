package route

import (
	"context"
	"errors"
	"testing"
	"time"

	"business-gateway/internal/backend"
	"business-gateway/internal/dedup"
	"business-gateway/internal/group"
)

type memoryGroups map[string]group.Binding

func (m memoryGroups) Get(groupID string) (group.Binding, bool) {
	binding, ok := m[groupID]
	return binding, ok
}
func (m memoryGroups) List() []group.Binding              { return nil }
func (m memoryGroups) Upsert(binding group.Binding) error { m[binding.GroupID] = binding; return nil }
func (m memoryGroups) Delete(groupID string) error        { delete(m, groupID); return nil }

type fakeBackend struct {
	query     backend.InventoryQuery
	inventory backend.Inventory
	queryErr  error
	healthErr error
	queries   int
}

func (f *fakeBackend) QueryInventory(_ context.Context, query backend.InventoryQuery) (backend.Inventory, error) {
	f.query = query
	f.queries++
	return f.inventory, f.queryErr
}

func (f *fakeBackend) Health(context.Context) error { return f.healthErr }

func newTestService(api *fakeBackend) *Service {
	groups := memoryGroups{
		"customer@chatroom": {GroupID: "customer@chatroom", Type: group.TypeCustomer, CustomerCode: "270", Enabled: true},
		"admin@chatroom":    {GroupID: "admin@chatroom", Type: group.TypeAdmin, Enabled: true},
	}
	return NewService(groups, api, dedup.NewMemoryCache(time.Hour), map[string]struct{}{"admin-wxid": {}}, true)
}

func baseRequest(groupID, sender, content string, messageID int64) Request {
	return Request{
		RobotWxID:  "robot-wxid",
		GroupID:    groupID,
		SenderWxID: sender,
		MessageID:  messageID,
		Content:    content,
		IsAtMe:     true,
	}
}

func TestCustomerInventoryAlwaysUsesBoundCustomer(t *testing.T) {
	api := &fakeBackend{inventory: backend.Inventory{CustomerCode: "270", Summary: backend.InventorySummary{Count: 1}}}
	service := newTestService(api)

	response := service.Route(context.Background(), baseRequest("customer@chatroom", "member", "@机器人\u2005查库存 300", 1))
	if !response.Handled || response.Error != "" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if api.query.CustomerCode != "270" || api.query.Keyword != "300" {
		t.Fatalf("query = %+v, want bound customer 270 and keyword 300", api.query)
	}
}

func TestCustomerCannotUseAdminInventorySyntax(t *testing.T) {
	api := &fakeBackend{}
	response := newTestService(api).Route(context.Background(), baseRequest("customer@chatroom", "member", "查 300 库存", 2))
	if !response.Handled || response.Error == "" || api.queries != 0 {
		t.Fatalf("cross-customer query was not blocked: response=%+v queries=%d", response, api.queries)
	}
}

func TestAdminInventoryRequiresBothGroupAndWxID(t *testing.T) {
	api := &fakeBackend{}
	service := newTestService(api)

	denied := service.Route(context.Background(), baseRequest("admin@chatroom", "ordinary-member", "查库存 300", 3))
	if !denied.Handled || denied.Error == "" || api.queries != 0 {
		t.Fatalf("ordinary member was not denied: %+v", denied)
	}

	api.inventory = backend.Inventory{CustomerCode: "300", Summary: backend.InventorySummary{Count: 2}}
	allowed := service.Route(context.Background(), baseRequest("admin@chatroom", "admin-wxid", "查库存 300 红", 4))
	if !allowed.Handled || allowed.Error != "" {
		t.Fatalf("authorized admin was denied: %+v", allowed)
	}
	if api.query.CustomerCode != "300" || api.query.Keyword != "红" {
		t.Fatalf("admin query = %+v", api.query)
	}
}

func TestExperimentalStatusOnlyAllowsAuthorizedAdmin(t *testing.T) {
	api := &fakeBackend{}
	service := newTestService(api)

	customer := service.Route(context.Background(), baseRequest("customer@chatroom", "member", "业务状态", 5))
	if !customer.Handled || customer.Error == "" {
		t.Fatalf("customer accessed experimental status: %+v", customer)
	}
	admin := service.Route(context.Background(), baseRequest("admin@chatroom", "admin-wxid", "业务状态", 6))
	if !admin.Handled || admin.Error != "" || admin.Reply == "" {
		t.Fatalf("admin status failed: %+v", admin)
	}
}

func TestRoutingSafetyRules(t *testing.T) {
	api := &fakeBackend{}
	service := newTestService(api)

	nonBusiness := service.Route(context.Background(), baseRequest("customer@chatroom", "member", "今天天气怎么样", 7))
	if nonBusiness.Handled {
		t.Fatalf("non-business message was intercepted: %+v", nonBusiness)
	}
	ownMessage := service.Route(context.Background(), baseRequest("customer@chatroom", "robot-wxid", "查库存", 71))
	if ownMessage.Handled {
		t.Fatalf("robot's own message was intercepted: %+v", ownMessage)
	}

	notAt := baseRequest("customer@chatroom", "member", "查库存", 8)
	notAt.IsAtMe = false
	if response := service.Route(context.Background(), notAt); response.Handled {
		t.Fatalf("message without mention was intercepted: %+v", response)
	}

	unbound := service.Route(context.Background(), baseRequest("unknown@chatroom", "member", "查库存", 9))
	if !unbound.Handled || unbound.Error == "" {
		t.Fatalf("unbound business message could fall through to AI: %+v", unbound)
	}

	api.queryErr = errors.New("database unavailable")
	failure := service.Route(context.Background(), baseRequest("customer@chatroom", "member", "查库存", 10))
	if !failure.Handled || failure.Error == "" {
		t.Fatalf("backend failure could fall through to AI: %+v", failure)
	}
}

func TestDuplicateBusinessMessageStopsWithoutSecondReply(t *testing.T) {
	api := &fakeBackend{inventory: backend.Inventory{CustomerCode: "270"}}
	service := newTestService(api)
	req := baseRequest("customer@chatroom", "member", "查库存", 11)

	first := service.Route(context.Background(), req)
	second := service.Route(context.Background(), req)
	if !first.Handled || !second.Handled || second.Reply != "" || second.Error != "" {
		t.Fatalf("unexpected duplicate responses: first=%+v second=%+v", first, second)
	}
	if api.queries != 1 {
		t.Fatalf("backend queried %d times, want 1", api.queries)
	}
}
