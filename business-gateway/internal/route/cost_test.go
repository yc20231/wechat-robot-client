package route

import (
	"context"
	"errors"
	"strings"
	"testing"

	"business-gateway/internal/backend"
)

func costFixture() backend.MaterialCost {
	price := "1.5"
	costValue := "1290"
	return backend.MaterialCost{
		Resolved:      true,
		Query:         "网的18厚白",
		CustomerCode:  "网",
		ArticleNumber: "18厚白",
		File:          &backend.MaterialCostFile{ID: 2, Name: "网的18厚白.xlsx", VersionNo: 3},
		Cost: &backend.MaterialCostSnapshot{
			Status:            "complete",
			TotalWeightJin:    "2012.5",
			KnownCost:         "8434.5",
			TotalCost:         strPtr("8434.5"),
			AverageCostPerJin: strPtr("4.191"),
			Regions: []backend.MaterialCostRegion{{
				Name:           "A",
				TotalWeightJin: "2012.5",
				Rows: []backend.MaterialCostRow{{
					MaterialName: "7000F", RawQuantity: "15", WeightJin: "300",
					UnitPrice: &price, Cost: &costValue,
				}},
			}},
		},
		Sheet: &backend.MaterialCostSheet{
			RowCount: 2, ColumnCount: 4, SheetName: "Sheet1",
			RowHeaderWidth: 46, ColumnHeaderHeight: 22,
			ColumnWidths: []float64{93.4, 149.4, 93.4, 149.4},
			RowHeights:   []float64{40, 30.8},
			Cells:        [][]string{{"阳强配料生产安排单", "", "", ""}, {"吹塑机号", "6", "客户代号", "网"}},
			CellStyles:   [][]string{{"s_title", "", "", ""}, {"s_label", "", "", ""}},
			Styles: map[string]backend.MaterialSheetStyle{
				"s_title": {FontFamily: "宋体", FontSize: 20, Bold: true, Align: "center"},
				"s_label": {FontFamily: "宋体", FontSize: 12, Align: "right"},
			},
			Merges: []backend.MaterialSheetMerge{{StartRow: 0, EndRow: 0, StartColumn: 0, EndColumn: 3}},
		},
	}
}

func strPtr(value string) *string { return &value }

func TestCostQueryDeniedForNonAdmin(t *testing.T) {
	api := &fakeBackend{}
	service := newTestService(api)
	response := service.Route(context.Background(), baseRequest("customer@chatroom", "member", "查成本 网的18厚白", 401))
	if !response.Handled || !strings.Contains(response.Error, "仅限机器人管理员") {
		t.Fatalf("non-admin was not denied: %+v", response)
	}
	if api.costCalls != 0 {
		t.Fatalf("backend queried %d times for non-admin", api.costCalls)
	}
}

func TestCostQueryNaturalPhrasingResolvesWithImage(t *testing.T) {
	api := &fakeBackend{cost: costFixture()}
	service := newTestService(api)
	response := service.Route(context.Background(), baseRequest("customer@chatroom", "admin-wxid", "查询网的18厚白配料成本是多少", 402))
	if response.Error != "" {
		t.Fatalf("unexpected error: %+v", response)
	}
	if api.costKeyword != "网的18厚白" || api.costFileID != 0 || api.costCalls != 1 {
		t.Fatalf("backend query keyword=%q fileID=%d calls=%d", api.costKeyword, api.costFileID, api.costCalls)
	}
	for _, want := range []string{"【网的18厚白】配料成本", "8382元/吨", "共计：2012.5斤 = 8434.5元", "7000F 15 → 300斤 = ¥1290"} {
		if !strings.Contains(response.Reply, want) {
			t.Fatalf("reply missing %q:\n%s", want, response.Reply)
		}
	}
	if response.Image == nil || response.Image.Title != "网的18厚白" || len(response.Image.Cells) != 2 {
		t.Fatalf("image = %+v", response.Image)
	}
	if len(response.Image.Merges) != 1 || response.Image.Merges[0].EndColumn != 3 {
		t.Fatalf("image merges = %+v", response.Image.Merges)
	}
	if response.Image.Cost == nil || response.Image.Cost.KnownCost != "8434.5" {
		t.Fatalf("image cost = %+v", response.Image.Cost)
	}
	if len(response.Image.ColumnWidths) != 4 || response.Image.ColumnWidths[0] != 93.4 {
		t.Fatalf("image column widths = %v", response.Image.ColumnWidths)
	}
	if response.Image.Styles["s_title"].FontSize != 20 {
		t.Fatalf("image styles = %+v", response.Image.Styles)
	}
}

func TestCostIncompleteReplyUsesKnownCost(t *testing.T) {
	api := &fakeBackend{cost: costFixture()}
	api.cost.Cost.Status = "incomplete"
	api.cost.Cost.TotalCost = nil
	api.cost.Cost.AverageCostPerJin = nil
	api.cost.Cost.KnownCost = "8417.5"
	api.cost.Cost.MissingMaterials = []string{"003"}
	service := newTestService(api)
	response := service.Route(context.Background(), baseRequest("customer@chatroom", "root-wxid", "查成本 网的18厚白", 423))
	if !strings.Contains(response.Reply, "共计：2010斤 = 8417.5元") && !strings.Contains(response.Reply, "共计：2012.5斤 = 8417.5元") {
		t.Fatalf("incomplete reply should keep known cost:\n%s", response.Reply)
	}
	if response.Image == nil || response.Image.Cost == nil || response.Image.Cost.KnownCost != "8417.5" {
		t.Fatalf("incomplete image cost = %+v", response.Image)
	}
}

func TestCostQueryPrefixForm(t *testing.T) {
	api := &fakeBackend{cost: costFixture()}
	service := newTestService(api)
	response := service.Route(context.Background(), baseRequest("admin@chatroom", "root-wxid", "查成本 网的18厚白", 403))
	if response.Error != "" || api.costKeyword != "网的18厚白" {
		t.Fatalf("response=%+v keyword=%q", response, api.costKeyword)
	}
}

func TestCostUsageWithoutKeyword(t *testing.T) {
	api := &fakeBackend{}
	service := newTestService(api)
	response := service.Route(context.Background(), baseRequest("customer@chatroom", "root-wxid", "查成本", 404))
	if !response.Handled || response.Error != "" || !strings.Contains(response.Reply, "查成本") {
		t.Fatalf("usage response = %+v", response)
	}
	if api.costCalls != 0 {
		t.Fatalf("backend queried %d times for usage", api.costCalls)
	}
}

func TestCostMatchesOfferNumberedSelection(t *testing.T) {
	api := &fakeBackend{cost: backend.MaterialCost{
		Resolved: false,
		Query:    "18厚白",
		Matches: []backend.MaterialCostMatch{
			{Index: 1, FileID: 11, Name: "网的18厚白.xlsx", CustomerCode: "网", ArticleNumber: "18厚白"},
			{Index: 2, FileID: 12, Name: "红方的18厚白.xlsx", CustomerCode: "红方", ArticleNumber: "18厚白"},
		},
	}}
	service := newTestService(api)

	first := service.Route(context.Background(), baseRequest("customer@chatroom", "root-wxid", "查成本 18厚白", 405))
	if !strings.Contains(first.Reply, "#1 网的18厚白.xlsx") || !strings.Contains(first.Reply, "查成本 #序号") {
		t.Fatalf("matches reply = %q", first.Reply)
	}

	api.cost = costFixture()
	second := service.Route(context.Background(), baseRequest("customer@chatroom", "root-wxid", "查成本 #2", 406))
	if api.costFileID != 12 {
		t.Fatalf("selection used fileID=%d, want 12", api.costFileID)
	}
	if second.Error != "" || second.Image == nil || !strings.Contains(second.Reply, "【网的18厚白】") {
		t.Fatalf("selection response = %+v", second)
	}
}

func TestCostSelectionWithoutListFails(t *testing.T) {
	api := &fakeBackend{}
	service := newTestService(api)
	response := service.Route(context.Background(), baseRequest("customer@chatroom", "root-wxid", "查成本 #3", 407))
	if response.Error == "" || api.costCalls != 0 {
		t.Fatalf("selection without list = %+v calls=%d", response, api.costCalls)
	}
}

func TestCostBackendErrorIsPassedThrough(t *testing.T) {
	api := &fakeBackend{costErr: &backend.BackendError{Message: "没找到这张配料单"}}
	service := newTestService(api)
	response := service.Route(context.Background(), baseRequest("customer@chatroom", "root-wxid", "查成本 不存在的", 408))
	if response.Error != "没找到这张配料单" {
		t.Fatalf("error = %q", response.Error)
	}
}

func TestCostGenericErrorIsMasked(t *testing.T) {
	api := &fakeBackend{costErr: errors.New("connection refused")}
	service := newTestService(api)
	response := service.Route(context.Background(), baseRequest("customer@chatroom", "root-wxid", "查成本 网的18厚白", 409))
	if response.Error != "配料成本查询暂时不可用，请稍后再试" {
		t.Fatalf("error = %q", response.Error)
	}
}

func TestCostQueryKeywordSpacesStripped(t *testing.T) {
	api := &fakeBackend{cost: costFixture()}
	service := newTestService(api)
	service.Route(context.Background(), baseRequest("customer@chatroom", "root-wxid", "查成本 网的 18 厚白 ", 420))
	if api.costKeyword != "网的18厚白" {
		t.Fatalf("keyword = %q, want 网的18厚白", api.costKeyword)
	}
}

func TestCostCustomerMismatchAsksForConfirmation(t *testing.T) {
	api := &fakeBackend{cost: costFixture()}
	api.cost.CustomerCode = "026"
	service := newTestService(api)

	first := service.Route(context.Background(), baseRequest("customer@chatroom", "root-wxid", "查成本 网的18厚白", 421))
	if first.Image != nil {
		t.Fatalf("mismatched customer must not ship the sheet directly: %+v", first)
	}
	if !strings.Contains(first.Reply, "没找到客户「网」") || !strings.Contains(first.Reply, "客户代号：026") || !strings.Contains(first.Reply, "查成本 #1") {
		t.Fatalf("mismatch reply = %q", first.Reply)
	}

	second := service.Route(context.Background(), baseRequest("customer@chatroom", "root-wxid", "查成本 #1", 422))
	if second.Error != "" || second.Image == nil {
		t.Fatalf("manual confirmation failed: %+v", second)
	}
}

func TestCostPerTonConversion(t *testing.T) {
	if got := costPerTonText(strPtr("4.191")); got != "8382" {
		t.Fatalf("cost per ton = %q, want 8382", got)
	}
	if got := costPerTonText(nil); got != "" {
		t.Fatalf("nil cost per ton = %q", got)
	}
}
