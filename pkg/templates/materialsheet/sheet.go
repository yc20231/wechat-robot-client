// Package materialsheet 把业务网关下发的配料单文本网格渲染成带边框的 HTML
// 表格，供截图服务生成发群图片；数据完全来自后端工作簿快照，不做任何计算。
package materialsheet

import (
	"fmt"
	"html"
	"strings"
)

// Merge 是合并单元格，坐标相对打印区域左上角，行列均为闭区间。
type Merge struct {
	StartRow    int
	EndRow      int
	StartColumn int
	EndColumn   int
}

// Render 渲染配料单网格；标题行（跨满整行的首个非空行）自动加粗放大。
func Render(title string, cells [][]string, merges []Merge) string {
	rows := len(cells)
	columns := 0
	for _, row := range cells {
		if len(row) > columns {
			columns = len(row)
		}
	}
	if rows == 0 || columns == 0 {
		return emptySheetHTML(title)
	}

	type span struct{ rows, columns int }
	covered := make(map[[2]int]bool)
	spans := make(map[[2]int]span)
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

	var builder strings.Builder
	builder.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><style>`)
	builder.WriteString(`body{margin:0;padding:20px;background:#fff;font-family:"Songti SC","SimSun","STSong","Noto Serif CJK SC",serif;color:#000;}`)
	fmt.Fprintf(&builder, `.sheet{border-collapse:collapse;table-layout:fixed;width:%dpx;}`, columns*180+40)
	builder.WriteString(`.sheet td{border:1px solid #000;min-height:32px;padding:4px 8px;font-size:15px;line-height:1.4;white-space:pre-wrap;word-break:break-all;text-align:center;vertical-align:middle;}`)
	builder.WriteString(`.title{font-size:20px;font-weight:700;}</style></head><body>`)
	builder.WriteString(`<table class="sheet">`)
	for r := 0; r < rows; r++ {
		builder.WriteString("<tr>")
		for c := 0; c < columns; c++ {
			if covered[[2]int{r, c}] {
				continue
			}
			text := ""
			if c < len(cells[r]) {
				text = cells[r][c]
			}
			var attrs strings.Builder
			if spanValue, ok := spans[[2]int{r, c}]; ok {
				if spanValue.rows > 1 {
					fmt.Fprintf(&attrs, ` rowspan="%d"`, spanValue.rows)
				}
				if spanValue.columns > 1 {
					fmt.Fprintf(&attrs, ` colspan="%d"`, spanValue.columns)
				}
				// 跨满整行的标题单元格按打印版式加粗放大。
				if spanValue.columns == columns && strings.TrimSpace(text) != "" {
					attrs.WriteString(` class="title"`)
				}
			}
			fmt.Fprintf(&builder, "<td%s>%s</td>", attrs.String(), html.EscapeString(text))
		}
		builder.WriteString("</tr>")
	}
	builder.WriteString("</table></body></html>")
	return builder.String()
}

func emptySheetHTML(title string) string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8"></head><body><p>` + html.EscapeString(title) + `</p></body></html>`
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
