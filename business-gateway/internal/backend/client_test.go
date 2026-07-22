package backend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestQueryInventoryUsesBotContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bot/inventory" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("X-Bot-Token") != "bot-secret" {
			t.Fatalf("missing bot token")
		}
		if got := r.Header.Get("User-Agent"); got != "bot-mcp/business-gateway-1.0" {
			t.Fatalf("User-Agent = %q", got)
		}
		if got := r.URL.Query().Get("customer_code"); got != "270" {
			t.Fatalf("customer_code = %q", got)
		}
		if got := r.URL.Query().Get("keyword"); got != "红" {
			t.Fatalf("keyword = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"customer_code":"270","summary":{"count":1},"items":[]}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "bot-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.QueryInventory(context.Background(), InventoryQuery{CustomerCode: "270", Keyword: "红"})
	if err != nil {
		t.Fatal(err)
	}
	if result.CustomerCode != "270" || result.Summary.Count != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestResolveCustomerUsesBotContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/bot/customers/resolve" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("customer_code"); got != "270" {
			t.Fatalf("customer_code = %q", got)
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"exists":true,"customer_code":"270","customer_name":"测试客户"}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "bot-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.ResolveCustomer(context.Background(), "270")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Exists || result.Code != "270" || result.Name != "测试客户" {
		t.Fatalf("unexpected result: %+v", result)
	}
}
