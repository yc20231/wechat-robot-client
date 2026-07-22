package route

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"business-gateway/internal/admin"
	"business-gateway/internal/audit"
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
	query      backend.InventoryQuery
	inventory  backend.Inventory
	queryErr   error
	healthErr  error
	queries    int
	customer   backend.Customer
	resolveErr error
}

func (f *fakeBackend) QueryInventory(_ context.Context, query backend.InventoryQuery) (backend.Inventory, error) {
	f.query = query
	f.queries++
	return f.inventory, f.queryErr
}

func (f *fakeBackend) Health(context.Context) error { return f.healthErr }

func (f *fakeBackend) ResolveCustomer(_ context.Context, customerCode string) (backend.Customer, error) {
	if f.resolveErr != nil {
		return backend.Customer{}, f.resolveErr
	}
	if f.customer.Code == "" {
		return backend.Customer{Exists: true, Code: customerCode, Name: "测试客户"}, nil
	}
	return f.customer, nil
}

type memoryAudit struct {
	mu     sync.Mutex
	events []audit.Event
}

func (m *memoryAudit) Record(event audit.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

type memoryAdmins struct {
	mu    sync.RWMutex
	roles map[string]admin.Role
}

func (m *memoryAdmins) RoleOf(wxID string) (admin.Role, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	role, ok := m.roles[wxID]
	return role, ok
}
func (m *memoryAdmins) IsOwner(wxID string) bool {
	role, ok := m.RoleOf(wxID)
	return ok && role == admin.RoleOwner
}
func (m *memoryAdmins) IsRoot(wxID string) bool {
	role, ok := m.RoleOf(wxID)
	return ok && (role == admin.RoleOwner || role == admin.RoleRoot)
}
func (m *memoryAdmins) IsAdmin(wxID string) bool { _, ok := m.RoleOf(wxID); return ok }
func (m *memoryAdmins) List() []admin.Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]admin.Entry, 0, len(m.roles))
	for wxID, role := range m.roles {
		result = append(result, admin.Entry{WxID: wxID, Role: role, Fixed: role == admin.RoleOwner})
	}
	return result
}
func (m *memoryAdmins) SetRole(wxID string, role admin.Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.roles[wxID] == admin.RoleOwner {
		return errors.New("固定所有者不可修改")
	}
	m.roles[wxID] = role
	return nil
}
func (m *memoryAdmins) DemoteRoot(wxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.roles[wxID] != admin.RoleRoot {
		return errors.New("目标不是动态根管理员")
	}
	m.roles[wxID] = admin.RoleAdmin
	return nil
}
func (m *memoryAdmins) Delete(wxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.roles[wxID] == admin.RoleOwner {
		return errors.New("固定所有者不可移除")
	}
	delete(m.roles, wxID)
	return nil
}

func newTestService(api *fakeBackend) *Service {
	groups := memoryGroups{
		"customer@chatroom": {GroupID: "customer@chatroom", Type: group.TypeCustomer, CustomerCode: "270", Enabled: true},
		"admin@chatroom":    {GroupID: "admin@chatroom", Type: group.TypeAdmin, Enabled: true},
	}
	admins := &memoryAdmins{roles: map[string]admin.Role{"owner-wxid": admin.RoleOwner, "root-wxid": admin.RoleRoot, "admin-wxid": admin.RoleAdmin}}
	return NewService(groups, api, dedup.NewMemoryCache(time.Hour), admins, nil, true, 5*time.Minute)
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

func managementRequest(groupID, sender, content, target string, messageID int64) Request {
	req := baseRequest(groupID, sender, content, messageID)
	req.MentionedWxIDs = []string{"robot-wxid"}
	if target != "" {
		req.MentionedWxIDs = append(req.MentionedWxIDs, target)
	}
	return req
}

func TestRootAdminsManageDynamicRolesWithConfirmation(t *testing.T) {
	service := newTestService(&fakeBackend{})
	auditLog := &memoryAudit{}
	service.audit = auditLog

	start := service.Route(context.Background(), managementRequest("unbound@chatroom", "root-wxid", "@机器人 添加 @W 管理员", "new-admin", 100))
	if !start.Handled || start.Error != "" || !strings.Contains(start.Reply, "确认添加") {
		t.Fatalf("start add admin = %+v", start)
	}
	confirm := service.Route(context.Background(), managementRequest("unbound@chatroom", "root-wxid", "@机器人 确认添加 @W 管理员", "new-admin", 101))
	if !confirm.Handled || confirm.Error != "" || !service.admins.IsAdmin("new-admin") {
		t.Fatalf("confirm add admin = %+v", confirm)
	}

	startRoot := service.Route(context.Background(), managementRequest("other@chatroom", "root-wxid", "添加 @R 根管理员", "new-root", 102))
	confirmRoot := service.Route(context.Background(), managementRequest("other@chatroom", "root-wxid", "确认添加 @R 根管理员", "new-root", 103))
	if startRoot.Error != "" || confirmRoot.Error != "" || !service.admins.IsRoot("new-root") {
		t.Fatalf("dynamic root creation failed: start=%+v confirm=%+v", startRoot, confirmRoot)
	}

	blockedOwnerRemoval := service.Route(context.Background(), managementRequest("other@chatroom", "new-root", "移除 @Y 根管理员", "owner-wxid", 104))
	if !blockedOwnerRemoval.Handled || blockedOwnerRemoval.Error == "" || !service.admins.IsOwner("owner-wxid") {
		t.Fatalf("owner removal was not blocked: %+v", blockedOwnerRemoval)
	}

	startDemote := service.Route(context.Background(), managementRequest("other@chatroom", "new-root", "移除 @R 根管理员", "root-wxid", 105))
	confirmDemote := service.Route(context.Background(), managementRequest("other@chatroom", "new-root", "确认移除 @R 根管理员", "root-wxid", 106))
	role, ok := service.admins.RoleOf("root-wxid")
	if startDemote.Error != "" || confirmDemote.Error != "" || !ok || role != admin.RoleAdmin {
		t.Fatalf("root demotion failed: start=%+v confirm=%+v role=%q", startDemote, confirmDemote, role)
	}
	if len(auditLog.events) != 3 {
		t.Fatalf("audit events = %d, want 3", len(auditLog.events))
	}
}

func TestRoleChangeRequiresRealSingleMentionAndSameActor(t *testing.T) {
	service := newTestService(&fakeBackend{})
	missingMention := service.Route(context.Background(), managementRequest("group@chatroom", "root-wxid", "添加 @W 管理员", "", 110))
	if missingMention.Error == "" {
		t.Fatalf("missing real mention was accepted: %+v", missingMention)
	}

	start := service.Route(context.Background(), managementRequest("group@chatroom", "root-wxid", "添加 @W 管理员", "target", 111))
	wrongActor := service.Route(context.Background(), managementRequest("group@chatroom", "owner-wxid", "确认添加 @W 管理员", "target", 112))
	if start.Error != "" || wrongActor.Error == "" || service.admins.IsAdmin("target") {
		t.Fatalf("different actor confirmed operation: start=%+v confirm=%+v", start, wrongActor)
	}
}

func TestCustomerBindingLifecycleAndPermissions(t *testing.T) {
	service := newTestService(&fakeBackend{})
	auditLog := &memoryAudit{}
	service.audit = auditLog
	groupID := "new-customer@chatroom"

	denied := service.Route(context.Background(), managementRequest(groupID, "admin-wxid", "绑定客户 270", "", 120))
	if denied.Error == "" {
		t.Fatalf("ordinary admin started binding: %+v", denied)
	}

	start := service.Route(context.Background(), managementRequest(groupID, "root-wxid", "绑定客户 270", "", 121))
	confirm := service.Route(context.Background(), managementRequest(groupID, "root-wxid", "确认绑定 270", "", 122))
	binding, ok := service.groups.Get(groupID)
	if start.Error != "" || confirm.Error != "" || !ok || binding.Type != group.TypeCustomer || binding.CustomerCode != "270" {
		t.Fatalf("customer bind failed: start=%+v confirm=%+v binding=%+v", start, confirm, binding)
	}

	startRebind := service.Route(context.Background(), managementRequest(groupID, "root-wxid", "改绑客户 365", "", 123))
	confirmRebind := service.Route(context.Background(), managementRequest(groupID, "root-wxid", "确认改绑 365", "", 124))
	binding, ok = service.groups.Get(groupID)
	if startRebind.Error != "" || confirmRebind.Error != "" || !ok || binding.CustomerCode != "365" {
		t.Fatalf("customer rebind failed: start=%+v confirm=%+v binding=%+v", startRebind, confirmRebind, binding)
	}

	startUnbind := service.Route(context.Background(), managementRequest(groupID, "root-wxid", "解绑客户", "", 125))
	confirmUnbind := service.Route(context.Background(), managementRequest(groupID, "root-wxid", "确认解绑客户 365", "", 126))
	if startUnbind.Error != "" || confirmUnbind.Error != "" {
		t.Fatalf("customer unbind failed: start=%+v confirm=%+v", startUnbind, confirmUnbind)
	}
	if _, ok := service.groups.Get(groupID); ok {
		t.Fatal("customer binding still exists after unbind")
	}
	if len(auditLog.events) != 3 {
		t.Fatalf("audit events = %d, want 3", len(auditLog.events))
	}
}

func TestAdminGroupBindingIsOwnerOnlyAndReversible(t *testing.T) {
	service := newTestService(&fakeBackend{})
	groupID := "new-admin@chatroom"
	denied := service.Route(context.Background(), managementRequest(groupID, "root-wxid", "绑定管理员群", "", 130))
	if denied.Error == "" {
		t.Fatalf("dynamic root bound admin group: %+v", denied)
	}

	start := service.Route(context.Background(), managementRequest(groupID, "owner-wxid", "绑定管理员群", "", 131))
	confirm := service.Route(context.Background(), managementRequest(groupID, "owner-wxid", "确认绑定管理员群", "", 132))
	binding, ok := service.groups.Get(groupID)
	if start.Error != "" || confirm.Error != "" || !ok || binding.Type != group.TypeAdmin {
		t.Fatalf("admin group bind failed: start=%+v confirm=%+v binding=%+v", start, confirm, binding)
	}

	startUnbind := service.Route(context.Background(), managementRequest(groupID, "owner-wxid", "解绑管理员群", "", 133))
	confirmUnbind := service.Route(context.Background(), managementRequest(groupID, "owner-wxid", "确认解绑管理员群", "", 134))
	if startUnbind.Error != "" || confirmUnbind.Error != "" {
		t.Fatalf("admin group unbind failed: start=%+v confirm=%+v", startUnbind, confirmUnbind)
	}
	if _, ok := service.groups.Get(groupID); ok {
		t.Fatal("admin group binding still exists after unbind")
	}
}

func TestConfirmationExpiresAndBusinessCommandsRemainFailClosed(t *testing.T) {
	service := newTestService(&fakeBackend{})
	clock := time.Unix(1000, 0)
	service.confirmations.now = func() time.Time { return clock }
	service.confirmationTTL = time.Minute

	start := service.Route(context.Background(), managementRequest("expiring@chatroom", "root-wxid", "绑定客户 270", "", 140))
	clock = clock.Add(2 * time.Minute)
	confirm := service.Route(context.Background(), managementRequest("expiring@chatroom", "root-wxid", "确认绑定 270", "", 141))
	if start.Error != "" || confirm.Error == "" {
		t.Fatalf("expired confirmation was accepted: start=%+v confirm=%+v", start, confirm)
	}

	unboundBusiness := service.Route(context.Background(), baseRequest("expiring@chatroom", "member", "查库存", 142))
	if !unboundBusiness.Handled || unboundBusiness.Error == "" {
		t.Fatalf("unbound business command fell through: %+v", unboundBusiness)
	}
}
