// Package materialsheet 把业务网关下发的配料单网格和成本快照渲染成
// 编辑器样式 HTML（左表右成本面板），供截图服务生成发群图片。
package materialsheet

import (
	"bytes"
	"embed"
	"html/template"
	"strconv"
	"strings"
)

//go:embed editor.html
var editorFS embed.FS

var editorTmpl = template.Must(template.New("editor.html").ParseFS(editorFS, "editor.html"))

// Merge 是合并单元格，坐标相对打印区域左上角，行列均为闭区间。
type Merge struct {
	StartRow    int
	EndRow      int
	StartColumn int
	EndColumn   int
}

// Document 是一张配料安排单截图所需的全部数据。
type Document struct {
	Title  string
	Cells  [][]string
	Merges []Merge
	Cost   *CostPanel
	Layout *SheetLayout
}

// SheetLayout 来自 Univer 工作簿的列宽、行高和单元格样式。
type SheetLayout struct {
	SheetName          string
	RowHeaderWidth     float64
	ColumnHeaderHeight float64
	ColumnWidths       []float64
	RowHeights         []float64
	CellStyles         [][]string
	Styles             map[string]CellStyle
}

// CellStyle 是截图用的字体/对齐。
type CellStyle struct {
	FontFamily string
	FontSize   float64
	Bold       bool
	Align      string
}

// CostPanel 对应编辑器右侧配料成本面板。
type CostPanel struct {
	Status            string
	TotalWeightJin    string
	KnownCost         string
	AverageCostPerJin *string
	ProcessingFee     *string
	MissingMaterials  []string
	UnsupportedRows   []UnsupportedRow
	Regions           []CostRegion
}

// CostRegion 是 A/B 等配料区域。
type CostRegion struct {
	Name           string
	TotalWeightJin string
	TotalCost      *string
	Rows           []CostRow
}

// CostRow 是区域中的一行原料。
type CostRow struct {
	MaterialName string
	RawQuantity  string
	WeightJin    string
	UnitPrice    *string
	Cost         *string
}

// UnsupportedRow 是无法换算数量的行。
type UnsupportedRow struct {
	MaterialName string
	RawQuantity  string
	Reason       string
}

type viewCell struct {
	Text    string
	ColSpan int
	RowSpan int
	Title   bool
	Wrap    bool
	Skip    bool
	CSS     template.CSS
}

type viewRow struct {
	Number int
	Height string
	Cells  []viewCell
}

type viewRegion struct {
	Name   string
	Weight string
	Cost   string
	Rows   []viewCostRow
}

type viewCostRow struct {
	Name    string
	Meta    string
	Value   string
	Missing bool
}

type editorView struct {
	Title              string
	SheetName          string
	Columns            []string
	ColumnWidths       []string
	RowHeaderWidth     string
	ColumnHeaderHeight string
	TableWidth         string
	Rows               []viewRow
	HasCost            bool
	Incomplete         bool
	TotalWeight        string
	KnownCost          string
	CostPerTon         string
	ProcessingFee      string
	Regions            []viewRegion
	Missing            []string
	Unsupported        []string
}

// Render 渲染编辑器样式的配料单+成本面板。
func Render(doc Document) string {
	view := buildView(doc)
	var buf bytes.Buffer
	if err := editorTmpl.Execute(&buf, view); err != nil {
		return "<!DOCTYPE html><html><head><meta charset=\"utf-8\"></head><body><p>" +
			template.HTMLEscapeString(doc.Title) + "</p></body></html>"
	}
	return buf.String()
}

func buildView(doc Document) editorView {
	view := editorView{
		Title:         strings.TrimSpace(doc.Title),
		SheetName:     "Sheet1",
		CostPerTon:    "—",
		ProcessingFee: "—",
		KnownCost:     "—",
	}
	rows, columns := gridSize(doc.Cells)
	layout := effectiveLayout(doc.Layout, rows, columns)
	if layout.SheetName != "" {
		view.SheetName = layout.SheetName
	}
	view.RowHeaderWidth = px(layout.RowHeaderWidth)
	view.ColumnHeaderHeight = px(layout.ColumnHeaderHeight)
	tableWidth := layout.RowHeaderWidth
	for _, width := range layout.ColumnWidths {
		view.ColumnWidths = append(view.ColumnWidths, px(width))
		tableWidth += width
	}
	view.TableWidth = px(tableWidth)
	if rows > 0 && columns > 0 {
		view.Columns = columnLetters(columns)
		view.Rows = buildSheetRows(doc.Cells, doc.Merges, layout, rows, columns)
	}
	if doc.Cost == nil {
		return view
	}
	panel := doc.Cost
	view.HasCost = true
	view.Incomplete = panel.Status == "incomplete"
	view.TotalWeight = formatNumber(panel.TotalWeightJin)
	if known := formatNumber(panel.KnownCost); known != "—" {
		view.KnownCost = known
	}
	if tons := costPerTonText(panel.AverageCostPerJin); tons != "" {
		view.CostPerTon = tons + "元/吨"
	}
	if fee := strings.TrimSpace(deref(panel.ProcessingFee)); fee != "" {
		view.ProcessingFee = "¥" + formatNumber(fee)
	}
	view.Missing = panel.MissingMaterials
	for _, row := range panel.UnsupportedRows {
		line := strings.TrimSpace(row.MaterialName + " " + row.RawQuantity)
		if row.Reason != "" {
			line = strings.TrimSpace(line + "：" + row.Reason)
		}
		if line != "" {
			view.Unsupported = append(view.Unsupported, line)
		}
	}
	for _, region := range panel.Regions {
		item := viewRegion{
			Name:   region.Name,
			Weight: formatNumber(region.TotalWeightJin),
			Cost:   money(region.TotalCost),
		}
		for _, row := range region.Rows {
			item.Rows = append(item.Rows, viewCostRow{
				Name:    fallback(row.MaterialName, "未识别原料"),
				Meta:    strings.TrimSpace(row.RawQuantity) + " · " + formatNumber(row.WeightJin) + "斤",
				Value:   rowValue(row),
				Missing: row.UnitPrice == nil || strings.TrimSpace(*row.UnitPrice) == "",
			})
		}
		view.Regions = append(view.Regions, item)
	}
	return view
}

func buildSheetRows(cells [][]string, merges []Merge, layout SheetLayout, rows, columns int) []viewRow {
	type span struct{ rows, columns int }
	covered := map[[2]int]bool{}
	spans := map[[2]int]span{}
	for _, merge := range merges {
		r0, r1 := clamp(merge.StartRow, 0, rows-1), clamp(merge.EndRow, 0, rows-1)
		c0, c1 := clamp(merge.StartColumn, 0, columns-1), clamp(merge.EndColumn, 0, columns-1)
		if r0 > r1 || c0 > c1 {
			continue
		}
		spans[[2]int{r0, c0}] = span{r1 - r0 + 1, c1 - c0 + 1}
		for r := r0; r <= r1; r++ {
			for c := c0; c <= c1; c++ {
				if r == r0 && c == c0 {
					continue
				}
				covered[[2]int{r, c}] = true
			}
		}
	}
	out := make([]viewRow, 0, rows)
	for r := 0; r < rows; r++ {
		row := viewRow{Number: r + 1, Cells: make([]viewCell, 0, columns)}
		if r < len(layout.RowHeights) {
			row.Height = px(layout.RowHeights[r])
		}
		for c := 0; c < columns; c++ {
			cell := viewCell{ColSpan: 1, RowSpan: 1, Skip: covered[[2]int{r, c}]}
			if !cell.Skip {
				if c < len(cells[r]) {
					cell.Text = cells[r][c]
				}
				style := lookupCellStyle(layout, r, c)
				cell.CSS = styleCSS(style, layout.rowHeight(r))
				if spanValue, ok := spans[[2]int{r, c}]; ok {
					cell.RowSpan = spanValue.rows
					cell.ColSpan = spanValue.columns
					if spanValue.columns == columns && strings.TrimSpace(cell.Text) != "" {
						cell.Title = style.FontSize == 0 || style.FontSize >= 16
					}
				}
				if style.Align == "left" || len([]rune(strings.TrimSpace(cell.Text))) > 16 {
					cell.Wrap = true
				}
			}
			row.Cells = append(row.Cells, cell)
		}
		out = append(out, row)
	}
	return out
}

func effectiveLayout(layout *SheetLayout, rows, columns int) SheetLayout {
	out := SheetLayout{
		SheetName:          "Sheet1",
		RowHeaderWidth:     46,
		ColumnHeaderHeight: 22,
	}
	if layout != nil {
		out = *layout
		if out.SheetName == "" {
			out.SheetName = "Sheet1"
		}
		if out.RowHeaderWidth <= 0 {
			out.RowHeaderWidth = 46
		}
		if out.ColumnHeaderHeight <= 0 {
			out.ColumnHeaderHeight = 22
		}
	}
	if len(out.ColumnWidths) < columns {
		grown := make([]float64, columns)
		copy(grown, out.ColumnWidths)
		for i := len(out.ColumnWidths); i < columns; i++ {
			if i%2 == 0 {
				grown[i] = 93.4
			} else {
				grown[i] = 149.4
			}
		}
		out.ColumnWidths = grown
	}
	if len(out.RowHeights) < rows {
		grown := make([]float64, rows)
		copy(grown, out.RowHeights)
		for i := len(out.RowHeights); i < rows; i++ {
			grown[i] = 30.8
			if i == 0 {
				grown[i] = 40
			}
			if i == 6 {
				grown[i] = 16
			}
		}
		out.RowHeights = grown
	}
	return out
}

func lookupCellStyle(layout SheetLayout, row, column int) CellStyle {
	if row >= len(layout.CellStyles) || column >= len(layout.CellStyles[row]) {
		return CellStyle{}
	}
	id := layout.CellStyles[row][column]
	if id == "" || layout.Styles == nil {
		return CellStyle{}
	}
	return layout.Styles[id]
}

func (layout SheetLayout) rowHeight(row int) float64 {
	if row >= 0 && row < len(layout.RowHeights) {
		return layout.RowHeights[row]
	}
	return 30.8
}

func styleCSS(style CellStyle, height float64) template.CSS {
	parts := make([]string, 0, 6)
	if height > 0 {
		parts = append(parts, "height:"+px(height))
	}
	family := strings.TrimSpace(style.FontFamily)
	if family == "" {
		family = "Songti SC"
	}
	parts = append(parts, "font-family:"+family+",SimSun,serif")
	size := style.FontSize
	if size <= 0 {
		size = 12
	}
	parts = append(parts, "font-size:"+strconv.FormatFloat(size, 'f', -1, 64)+"px")
	if style.Bold {
		parts = append(parts, "font-weight:700")
	}
	align := style.Align
	if align == "" {
		align = "center"
	}
	parts = append(parts, "text-align:"+align)
	return template.CSS(strings.Join(parts, ";"))
}

func px(value float64) string {
	if value <= 0 {
		return "0"
	}
	return strconv.FormatFloat(value, 'f', -1, 64) + "px"
}

func gridSize(cells [][]string) (rows, columns int) {
	rows = len(cells)
	for _, row := range cells {
		if len(row) > columns {
			columns = len(row)
		}
	}
	return rows, columns
}

func columnLetters(count int) []string {
	letters := make([]string, count)
	for i := 0; i < count; i++ {
		letters[i] = columnLetter(i)
	}
	return letters
}

func columnLetter(index int) string {
	if index < 0 {
		return ""
	}
	var builder strings.Builder
	for index >= 0 {
		builder.WriteByte(byte('A' + index%26))
		index = index/26 - 1
		if index < 0 {
			break
		}
	}
	runes := []rune(builder.String())
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func costPerTonText(averageCostPerJin *string) string {
	if averageCostPerJin == nil || strings.TrimSpace(*averageCostPerJin) == "" {
		return ""
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(*averageCostPerJin), 64)
	if err != nil || value <= 0 {
		return ""
	}
	return formatNumber(strconv.FormatFloat(value*2000, 'f', -1, 64))
}

func formatNumber(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return value
	}
	return strconv.FormatFloat(number, 'f', -1, 64)
}

func money(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "—"
	}
	return "¥ " + formatNumber(*value)
}

func rowValue(row CostRow) string {
	if row.UnitPrice == nil || strings.TrimSpace(*row.UnitPrice) == "" {
		return "缺少价格"
	}
	if row.Cost == nil || strings.TrimSpace(*row.Cost) == "" {
		return "—"
	}
	return "¥" + formatNumber(*row.Cost)
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func fallback(value, or string) string {
	if strings.TrimSpace(value) == "" {
		return or
	}
	return value
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
