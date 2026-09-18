package materialsheet

import (
	"strings"
	"testing"
)

func TestRenderBuildsTableWithMergesAndEscaping(t *testing.T) {
	cells := [][]string{
		{"阳强配料生产安排单", "", "", ""},
		{"吹塑机号", "6", "客户代号", "网"},
	}
	merges := []Merge{{StartRow: 0, EndRow: 0, StartColumn: 0, EndColumn: 3}}
	html := Render("网的18厚白", cells, merges)

	for _, want := range []string{
		`<td colspan="4" class="title">阳强配料生产安排单</td>`,
		`<td>吹塑机号</td>`,
		`<td>网</td>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("html missing %q:\n%s", want, html)
		}
	}
	// 合并被覆盖的单元格不应重复输出：标题 1 个 + 明细行 4 个
	if strings.Count(html, "<td") != 5 {
		t.Fatalf("cell count = %d, want 5:\n%s", strings.Count(html, "<td"), html)
	}
}

func TestRenderEscapesHTMLInCellText(t *testing.T) {
	html := Render("x", [][]string{{"<A>&B"}}, nil)
	if strings.Contains(html, "<A>&B") || !strings.Contains(html, "&lt;A&gt;&amp;B") {
		t.Fatalf("cell text not escaped:\n%s", html)
	}
}

func TestRenderEmptyGridFallsBackToTitle(t *testing.T) {
	html := Render("空的", nil, nil)
	if !strings.Contains(html, "空的") {
		t.Fatalf("empty fallback missing title:\n%s", html)
	}
}
