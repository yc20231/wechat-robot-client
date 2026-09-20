package materialsheet

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"wechat-robot-client/service"
)

func fortyHouBaiDocument() Document {
	row := func(name, qty, jin, price, cost string) CostRow {
		item := CostRow{MaterialName: name, RawQuantity: qty, WeightJin: jin}
		if price != "" {
			item.UnitPrice = strPtr(price)
		}
		if cost != "" {
			item.Cost = strPtr(cost)
		}
		return item
	}
	return Document{
		Title: "40厚白",
		Cells: [][]string{
			{"阳强配料生产安排单", "", "", ""},
			{"", "", "年 月 日", ""},
			{"吹塑机号", "6", "客户代号", "网"},
			{"配料工号", "65公斤", "货号", "40厚白"},
			{"产 量", "1000斤", "品 名", "细花白"},
			{"备 注", "", "规 格", "40+10*65=2.81"},
			{"", "", "", ""},
			{"A:", "", "B:", ""},
			{"7000F", "15", "", ""},
			{"55110", "10", "", ""},
			{"5502", "15", "", ""},
			{"6095", "15", "", ""},
			{"6888", "20", "", ""},
			{"原白冲头自己", "20", "", ""},
			{"原白冲头祖意", "5", "", ""},
			{"", "", "", ""},
			{"00952", "5斤", "", ""},
			{"0474", "5斤", "", ""},
			{"0474", "2.5斤", "", ""},
			{"", "", "", ""},
			{"请严格按照本单要求配料。注意各种料米比例，不得超重。未经", "", "", ""},
			{"管理人员确定不得私自更改，违者罚款。", "", "", ""},
		},
		Merges: []Merge{
			{StartRow: 0, EndRow: 0, StartColumn: 0, EndColumn: 3},
			{StartRow: 20, EndRow: 20, StartColumn: 0, EndColumn: 3},
			{StartRow: 21, EndRow: 21, StartColumn: 0, EndColumn: 1},
		},
		Layout: &SheetLayout{
			SheetName:          "Sheet1",
			RowHeaderWidth:     46,
			ColumnHeaderHeight: 22,
			ColumnWidths:       []float64{93.4, 149.4, 93.4, 149.4},
			RowHeights: []float64{
				40, 30.8, 30.8, 30.8, 30.8, 30.8, 16, 30.8, 30.8, 30.8,
				30.8, 30.8, 30.8, 30.8, 30.8, 30.8, 30.8, 30.8, 30.8, 30.8, 30.8, 30.8,
			},
			CellStyles: [][]string{{"s_title"}},
			Styles: map[string]CellStyle{
				"s_title": {FontFamily: "宋体", FontSize: 20, Bold: true, Align: "center"},
			},
		},
		Cost: &CostPanel{
			Status:            "complete",
			TotalWeightJin:    "2012.5",
			KnownCost:         "8434.5",
			AverageCostPerJin: strPtr("4.191"),
			Regions: []CostRegion{{
				Name:           "A",
				TotalWeightJin: "2012.5",
				TotalCost:      strPtr("8434.5"),
				Rows: []CostRow{
					row("7000F", "15", "300", "4.3", "1290"),
					row("55110", "10", "200", "4.25", "850"),
					row("5502", "15", "300", "4.2", "1260"),
					row("6095", "15", "300", "4.4", "1320"),
					row("6888", "20", "400", "4.3", "1720"),
					row("原白冲头自己", "20", "400", "3.9", "1560"),
					row("原白冲头祖意", "5", "100", "3.7", "370"),
					row("00952", "5斤", "5", "4.65", "23.25"),
					row("0474", "5斤", "5", "5.5", "27.5"),
					row("0474", "2.5斤", "2.5", "5.5", "13.75"),
				},
			}},
		},
	}
}

func TestWriteEditorReplicaFixture(t *testing.T) {
	html := Render(fortyHouBaiDocument())
	dir := filepath.Join("..", "..", "..", "output")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "cost-editor-replica.html")
	if err := os.WriteFile(path, []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%d bytes)", path, len(html))
}

func TestCaptureEditorReplica(t *testing.T) {
	captureDoc(t, "cost-editor-replica.png", fortyHouBaiDocument())
}

func captureDoc(t *testing.T, name string, doc Document) {
	t.Helper()
	html := Render(doc)
	png, err := service.CaptureHTMLScreenshotWidth(context.Background(), html, 1540)
	if err != nil {
		t.Skipf("chrome screenshot unavailable: %v", err)
	}
	dir := filepath.Join("..", "..", "..", "output")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, png, 0o644); err != nil {
		t.Fatal(err)
	}
	if len(png) < 20_000 {
		t.Fatalf("screenshot too small: %d bytes", len(png))
	}
	t.Logf("wrote %s (%d bytes)", path, len(png))
}
