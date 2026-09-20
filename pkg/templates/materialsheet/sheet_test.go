package materialsheet

import (
	"strings"
	"testing"
)

func strPtr(value string) *string { return &value }

func sampleDocument() Document {
	price := "4.3"
	cost := "1290"
	return Document{
		Title: "40厚白",
		Cells: [][]string{
			{"阳强配料生产安排单", "", "", ""},
			{"", "", "年 月 日", ""},
			{"吹塑机号", "6", "客户代号", "网"},
			{"配料工号", "65公斤", "货号", "40厚白"},
		},
		Merges: []Merge{{StartRow: 0, EndRow: 0, StartColumn: 0, EndColumn: 3}},
		Cost: &CostPanel{
			Status:            "complete",
			TotalWeightJin:    "2012.5",
			KnownCost:         "8434.5",
			AverageCostPerJin: strPtr("4.191"),
			Regions: []CostRegion{{
				Name:           "A",
				TotalWeightJin: "2012.5",
				TotalCost:      strPtr("8434.5"),
				Rows: []CostRow{{
					MaterialName: "7000F", RawQuantity: "15", WeightJin: "300",
					UnitPrice: &price, Cost: &cost,
				}},
			}},
		},
	}
}

func TestRenderBuildsTableWithMergesAndEscaping(t *testing.T) {
	html := Render(Document{
		Title:  "网的18厚白",
		Cells:  [][]string{{"阳强配料生产安排单", "", "", ""}, {"吹塑机号", "6", "客户代号", "网"}},
		Merges: []Merge{{StartRow: 0, EndRow: 0, StartColumn: 0, EndColumn: 3}},
	})

	for _, want := range []string{
		`colspan="4"`,
		`阳强配料生产安排单`,
		`吹塑机号`,
		`网`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("html missing %q:\n%s", want, html)
		}
	}
}

func TestRenderEscapesHTMLInCellText(t *testing.T) {
	html := Render(Document{Title: "x", Cells: [][]string{{"<A>&B"}}})
	if strings.Contains(html, "<A>&B") || !strings.Contains(html, "&lt;A&gt;&amp;B") {
		t.Fatalf("cell text not escaped:\n%s", html)
	}
}

func TestRenderEmptyGridFallsBackToTitle(t *testing.T) {
	html := Render(Document{Title: "空的"})
	if !strings.Contains(html, "空的") {
		t.Fatalf("empty fallback missing title:\n%s", html)
	}
}

func TestRenderReplicasEditorLayout(t *testing.T) {
	html := Render(sampleDocument())
	for _, want := range []string{
		"撤销", "重做", "字体", "宋体", "字号", "12",
		"toolbar-separator",
		">A</", ">B</", ">C</", ">D</",
		">1</", ">2</",
		"row-num",
		"配料成本", "实时预览",
		"共计", "2012.5斤", "8434.5元",
		"8382元/吨",
		"加工费",
		"A区域", "2012.5 斤",
		"7000F", "15 · 300斤", "¥1290",
		"Sheet1",
		"100%",
		"univer-shell",
		"93.4px",
		"149.4px",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("editor replica missing %q:\n%s", want, html)
		}
	}
}

func TestRenderUsesWorkbookCanvasLayout(t *testing.T) {
	html := Render(Document{
		Title: "40厚白",
		Cells: [][]string{{"阳强配料生产安排单", "", "", ""}},
		Layout: &SheetLayout{
			SheetName:      "Sheet1",
			RowHeaderWidth: 46,
			ColumnWidths:   []float64{93.4, 149.4, 93.4, 149.4},
			RowHeights:     []float64{40},
			CellStyles:     [][]string{{"s_title", "", "", ""}},
			Styles: map[string]CellStyle{
				"s_title": {FontFamily: "宋体", FontSize: 20, Bold: true, Align: "center"},
			},
		},
	})
	for _, want := range []string{
		"font-size:20px",
		"font-weight:700",
		"height:40px",
		"width:93.4px",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("canvas layout missing %q:\n%s", want, html)
		}
	}
}

func TestRenderIncompleteCostUsesKnownCostAndMissingPrice(t *testing.T) {
	html := Render(Document{
		Title: "18厚白",
		Cells: [][]string{{"阳强配料生产安排单", "", "", ""}},
		Cost: &CostPanel{
			Status:           "incomplete",
			TotalWeightJin:   "2010",
			KnownCost:        "8417.5",
			MissingMaterials: []string{"003"},
			Regions: []CostRegion{{
				Name:           "A",
				TotalWeightJin: "2010",
				Rows: []CostRow{
					{MaterialName: "0474", RawQuantity: "5斤", WeightJin: "5", Cost: strPtr("27.5"), UnitPrice: strPtr("5.5")},
					{MaterialName: "003", RawQuantity: "5斤", WeightJin: "5"},
				},
			}},
		},
	})
	for _, want := range []string{
		"部分原料尚未设置价格，成本仅供参考",
		"8417.5元",
		"缺少价格",
		"003",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("incomplete panel missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "共计") && strings.Contains(html, "= 一元") {
		t.Fatalf("incomplete total still uses empty total_cost:\n%s", html)
	}
}
