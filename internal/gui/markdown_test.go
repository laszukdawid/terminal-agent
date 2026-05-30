package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

func TestApplyStrikethrough(t *testing.T) {
	got := applyStrikethrough("ab")
	want := "a̶b̶"
	if got != want {
		t.Fatalf("applyStrikethrough(\"ab\") = %q, want %q", got, want)
	}
	if applyStrikethrough("") != "" {
		t.Fatalf("applyStrikethrough(\"\") should be empty")
	}
}

// textOf concatenates the plain text of every text-like segment so tests can
// assert on rendered content without depending on segment ordering details.
func textOf(segs []widget.RichTextSegment) string {
	var b strings.Builder
	for _, s := range segs {
		switch seg := s.(type) {
		case *widget.TextSegment:
			b.WriteString(seg.Text)
		case *widget.HyperlinkSegment:
			b.WriteString(seg.Text)
		case *widget.ListSegment:
			b.WriteString(textOf(seg.Items))
		case *widget.ParagraphSegment:
			b.WriteString(textOf(seg.Texts))
		case *codeBlockSegment:
			b.WriteString(seg.Code)
		}
	}
	return b.String()
}

func findCodeBlock(segs []widget.RichTextSegment) *codeBlockSegment {
	for _, s := range segs {
		switch seg := s.(type) {
		case *codeBlockSegment:
			return seg
		case *widget.ListSegment:
			if cb := findCodeBlock(seg.Items); cb != nil {
				return cb
			}
		case *widget.ParagraphSegment:
			if cb := findCodeBlock(seg.Texts); cb != nil {
				return cb
			}
		}
	}
	return nil
}

func TestRenderMarkdownStrikethrough(t *testing.T) {
	segs := renderMarkdown("This is ~~gone~~ text.")
	out := textOf(segs)
	if !strings.Contains(out, "g̶o̶n̶e̶") {
		t.Fatalf("strikethrough not applied, got %q", out)
	}
}

func TestRenderMarkdownTaskList(t *testing.T) {
	segs := renderMarkdown("- [x] done\n- [ ] todo\n")
	out := textOf(segs)
	if !strings.Contains(out, "☑") {
		t.Errorf("checked task glyph missing, got %q", out)
	}
	if !strings.Contains(out, "☐") {
		t.Errorf("unchecked task glyph missing, got %q", out)
	}
}

func TestRenderMarkdownCodeBlock(t *testing.T) {
	segs := renderMarkdown("```go\nfmt.Println(\"hi\")\n```\n")
	cb := findCodeBlock(segs)
	if cb == nil {
		t.Fatal("expected a codeBlockSegment")
	}
	if cb.Inline() {
		t.Error("code block should not be inline")
	}
	if !strings.Contains(cb.Code, `fmt.Println("hi")`) {
		t.Errorf("code block text wrong: %q", cb.Code)
	}
}

func TestRenderMarkdownHeadingAndLink(t *testing.T) {
	segs := renderMarkdown("# Title\n\n[site](https://example.com)\n")

	var heading *widget.TextSegment
	var found bool
	for _, s := range segs {
		if ts, ok := s.(*widget.TextSegment); ok && ts.Style.SizeName == widget.RichTextStyleHeading.SizeName && ts.Text == "Title" {
			heading = ts
			found = true
		}
	}
	if !found || heading == nil {
		t.Errorf("heading segment not found in %#v", segs)
	}

	if !strings.Contains(textOf(segs), "site") {
		t.Errorf("link text missing")
	}
}

func findTable(segs []widget.RichTextSegment) *tableSegment {
	for _, s := range segs {
		if tbl, ok := s.(*tableSegment); ok {
			return tbl
		}
	}
	return nil
}

func TestRenderMarkdownTable(t *testing.T) {
	md := "| Feature | Value |\n| :--- | ---: |\n| Bold | **x** |\n| Code | `y` |\n"
	tbl := findTable(renderMarkdown(md))
	if tbl == nil {
		t.Fatal("expected a tableSegment")
	}
	if tbl.Inline() {
		t.Error("table should not be inline")
	}
	if got := tbl.cols(); got != 2 {
		t.Errorf("cols = %d, want 2", got)
	}
	if len(tbl.header) != 2 {
		t.Fatalf("header cells = %d, want 2", len(tbl.header))
	}
	if len(tbl.rows) != 2 {
		t.Fatalf("body rows = %d, want 2", len(tbl.rows))
	}

	// Per-column alignment from the delimiter row.
	if tbl.alignFor(0) != fyne.TextAlignLeading {
		t.Errorf("col 0 align = %v, want leading", tbl.alignFor(0))
	}
	if tbl.alignFor(1) != fyne.TextAlignTrailing {
		t.Errorf("col 1 align = %v, want trailing", tbl.alignFor(1))
	}

	// Inline formatting inside cells is preserved as styled segments.
	var headerText string
	for _, cell := range tbl.header {
		headerText += textOf(cell)
	}
	if !strings.Contains(headerText, "Feature") || !strings.Contains(headerText, "Value") {
		t.Errorf("header text missing: %q", headerText)
	}
}

// TestRenderMarkdownTableRichCells verifies inline markdown inside table cells
// is rendered as styled segments (strong, code, hyperlink), not literal text.
func TestRenderMarkdownTableRichCells(t *testing.T) {
	md := "| A | B |\n| - | - |\n| **bold** | [site](https://example.com) |\n| `code` | plain |\n"
	tbl := findTable(renderMarkdown(md))
	if tbl == nil {
		t.Fatal("expected a tableSegment")
	}

	var hasStrong, hasCode, hasLink bool
	for _, row := range tbl.rows {
		for _, cell := range row {
			for _, s := range cell {
				switch seg := s.(type) {
				case *widget.TextSegment:
					if seg.Style.TextStyle.Bold {
						hasStrong = true
					}
					if seg.Style.TextStyle.Monospace {
						hasCode = true
					}
				case *widget.HyperlinkSegment:
					hasLink = true
				}
			}
		}
	}
	if !hasStrong {
		t.Error("bold cell did not produce a strong/bold segment")
	}
	if !hasCode {
		t.Error("code cell did not produce a monospace segment")
	}
	if !hasLink {
		t.Error("link cell did not produce a hyperlink segment")
	}
}

func TestTableLayoutFillsWidth(t *testing.T) {
	// Two empty cells in one row; layout should distribute slack so the cells
	// span the available width rather than hugging content.
	c1 := widget.NewLabel("a")
	c2 := widget.NewLabel("b")
	l := &tableLayout{cols: 2, gap: 1}
	objs := []fyne.CanvasObject{c1, c2}
	l.Layout(objs, fyne.NewSize(200, 40))

	// c2 must start to the right of c1, and the row must use most of the width.
	if c2.Position().X <= c1.Position().X {
		t.Errorf("second cell not placed after first: %v vs %v", c2.Position().X, c1.Position().X)
	}
	rightEdge := c2.Position().X + c2.Size().Width
	if rightEdge < 190 {
		t.Errorf("table did not fill width, right edge at %v", rightEdge)
	}
}
