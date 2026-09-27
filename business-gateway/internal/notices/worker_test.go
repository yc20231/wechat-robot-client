package notices

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"business-gateway/internal/group"
)

type memoryGroups []group.Binding

func (m memoryGroups) List() []group.Binding { return m }
func (m memoryGroups) Get(id string) (group.Binding, bool) {
	for _, b := range m {
		if b.GroupID == id {
			return b, true
		}
	}
	return group.Binding{}, false
}
func (m memoryGroups) Upsert(group.Binding) error { return nil }
func (m memoryGroups) Delete(string) error        { return nil }

func TestWorkerSequenceAndFailClosed(t *testing.T) {
	for _, scenario := range []struct {
		name                        string
		robotFail, ackFail, changed bool
		count                       int
		expected                    []string
	}{
		{name: "ordered", expected: []string{"begin0", "send0", "sent0", "begin1", "send1", "sent1"}},
		{name: "robot failure", robotFail: true, expected: []string{"begin0", "send0", "unknown0"}},
		{name: "ack lost", ackFail: true, expected: []string{"begin0", "send0", "sent0"}},
		{name: "binding changed", changed: true, expected: []string{"blocked0"}},
		{name: "resume only second", count: 1, expected: []string{"begin1", "send1", "sent1"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			events := []string{}
			sent := scenario.count
			robot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Production-Notice-Token") != strings.Repeat("r", 32) {
					t.Error("missing robot credential")
				}
				if strings.HasSuffix(r.URL.Path, "health") {
					json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"ready": true}})
					return
				}
				events = append(events, "send"+string(rune('0'+sent)))
				sent++
				if scenario.robotFail {
					w.WriteHeader(502)
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"accepted": true}})
			}))
			defer robot.Close()
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Bot-Token") != strings.Repeat("b", 32) || !strings.HasPrefix(r.Header.Get("User-Agent"), "bot-mcp/") {
					t.Error("missing ERP credential")
				}
				var data any
				if strings.HasSuffix(r.URL.Path, "claim") {
					data = map[string]any{"id": 1, "lease_token": "lease", "sent_count": scenario.count, "payload": map[string]any{"customer_code": "01", "group_id": "test@chatroom", "messages": []string{"first", "second"}}}
				}
				if strings.HasSuffix(r.URL.Path, "begin") || strings.HasSuffix(r.URL.Path, "result") {
					var step struct {
						Index   int    `json:"index"`
						Outcome string `json:"outcome"`
					}
					json.NewDecoder(r.Body).Decode(&step)
					action := step.Outcome
					if action == "" {
						action = "begin"
					}
					events = append(events, action+string(rune('0'+step.Index)))
					if scenario.ackFail && action == "sent" {
						w.WriteHeader(503)
						return
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
			}))
			defer backend.Close()
			code := "01"
			if scenario.changed {
				code = "02"
			}
			worker, err := New(Config{BackendURL: backend.URL, BackendToken: strings.Repeat("b", 32), RobotURL: robot.URL, RobotToken: strings.Repeat("r", 32), AllowedGroups: []string{"test@chatroom"}}, memoryGroups{{GroupID: "test@chatroom", Type: group.TypeCustomer, CustomerCode: code, Enabled: true}})
			if err != nil {
				t.Fatal(err)
			}
			_ = worker.Poll(context.Background())
			if !reflect.DeepEqual(events, scenario.expected) {
				t.Fatalf("events=%v expected=%v", events, scenario.expected)
			}
		})
	}
}
func TestWorkerRequiresExplicitTestAllowlist(t *testing.T) {
	_, err := New(Config{BackendURL: "http://127.0.0.1", RobotURL: "http://127.0.0.1", BackendToken: strings.Repeat("b", 32), RobotToken: strings.Repeat("r", 32)}, memoryGroups{})
	if err == nil {
		t.Fatal("empty allowlist enabled")
	}
}
