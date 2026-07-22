package route

import (
	"sync"
	"time"
)

type operationAction string

const (
	actionAddRoot          operationAction = "add_root"
	actionRemoveRoot       operationAction = "remove_root"
	actionAddAdmin         operationAction = "add_admin"
	actionRemoveAdmin      operationAction = "remove_admin"
	actionBindCustomer     operationAction = "bind_customer"
	actionRebindCustomer   operationAction = "rebind_customer"
	actionUnbindCustomer   operationAction = "unbind_customer"
	actionBindAdminGroup   operationAction = "bind_admin_group"
	actionRebindAdminGroup operationAction = "rebind_admin_group"
	actionUnbindAdminGroup operationAction = "unbind_admin_group"
)

type pendingOperation struct {
	Action       operationAction
	ActorWxID    string
	GroupID      string
	TargetWxID   string
	CustomerCode string
	CustomerName string
	ExpiresAt    time.Time
}

type confirmationStore struct {
	mu   sync.Mutex
	data map[string]pendingOperation
	now  func() time.Time
}

func newConfirmationStore() *confirmationStore {
	return &confirmationStore{data: make(map[string]pendingOperation), now: time.Now}
}

func (s *confirmationStore) put(operation pendingOperation, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	operation.ExpiresAt = s.now().Add(ttl)
	s.data[confirmationKey(operation.GroupID, operation.ActorWxID)] = operation
}

func (s *confirmationStore) take(groupID, actorWxID string, action operationAction) (pendingOperation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	key := confirmationKey(groupID, actorWxID)
	operation, ok := s.data[key]
	if !ok || operation.Action != action {
		return pendingOperation{}, false
	}
	delete(s.data, key)
	return operation, true
}

func (s *confirmationStore) pruneLocked() {
	now := s.now()
	for key, operation := range s.data {
		if !operation.ExpiresAt.After(now) {
			delete(s.data, key)
		}
	}
}

func confirmationKey(groupID, actorWxID string) string {
	return groupID + "\x00" + actorWxID
}
