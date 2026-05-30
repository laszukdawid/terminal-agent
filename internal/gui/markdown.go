package gui

// This file is a local, temporary replacement for Fyne's built-in markdown
// renderer (fyne.io/fyne/v2/widget.parseMarkdown). Fyne parses markdown with a
// vanilla goldmark instance and no GFM extensions, so strikethrough and task
// lists pass through as literal text, and fenced code blocks are rendered as
// plain monospace with no visual separation.
//
// renderMarkdown below mirrors Fyne's node handling but enables the GFM
// Strikethrough and TaskList extensions and adds a bordered panel for code
// blocks. It deliberately produces only public widget.RichTextSegment values
// (plus one small custom segment) so it can be swapped back out for an upstream
// fix with no other code changes.
//
// Upstream tracking issue: https://github.com/fyne-io/fyne/issues/6330
// Once Fyne renders these features natively, delete this file and call
// widget.RichText.ParseMarkdown directly again.

import (
	"html"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// strikeOverlay is the Unicode "combining long stroke overlay" (U+0336).
// Fyne's TextStyle has no strikethrough attribute, so we draw the line by
// appending this combining mark after every rune of a strikethrough run.
const strikeOverlay = '̶'

// renderMarkdown parses markdown (with GFM strikethrough, task lists and
// tables) into RichText segments. It is the in-repo replacement for
// widget.parseMarkdown.
func renderMarkdown(content string) []widget.RichTextSegment {
	md := goldmark.New(goldmark.WithExtensions(extension.Strikethrough, extension.TaskList, extension.Table))
	source := []byte(content)
	doc := md.Parser().Parse(text.NewReader(source))
	segs, _ := renderMarkdownNode(source, doc, false, 0)
	return segs
}

func renderMarkdownNode(source []byte, n ast.Node, blockquote bool, listDepth int) ([]widget.RichTextSegment, error) {
	switch t := n.(type) {
	case *ast.Document:
		return renderMarkdownChildren(source, n, blockquote, listDepth)
	case *ast.Paragraph:
		children, err := renderMarkdownChildren(source, n, blockquote, listDepth)
		if !blockquote {
			children = append(children, &widget.TextSegment{Style: widget.RichTextStyleParagraph})
		}
		return children, err
	case *ast.List:
		items, err := renderMarkdownChildren(source, n, blockquote, listDepth+1)
		return []widget.RichTextSegment{
			&widget.ListSegment{
				Items:   items,
				Ordered: t.Marker != '*' && t.Marker != '-' && t.Marker != '+',
			},
		}, err
	case *ast.ListItem:
		children, err := renderMarkdownChildren(source, n, blockquote, listDepth)
		var texts []widget.RichTextSegment
		var sublist widget.RichTextSegment
		for _, child := range children {
			if _, ok := child.(*widget.ListSegment); ok {
				sublist = child
			} else {
				texts = append(texts, child)
			}
		}
		result := []widget.RichTextSegment{&widget.ParagraphSegment{Texts: texts}}
		if sublist != nil {
			result = append(result, sublist)
		}
		return result, err
	case *ast.TextBlock:
		return renderMarkdownChildren(source, n, blockquote, listDepth)
	case *ast.Heading:
		text := decodeMarkdownText(forceIntoHeadingText(source, n))
		switch t.Level {
		case 1:
			return []widget.RichTextSegment{&widget.TextSegment{Style: widget.RichTextStyleHeading, Text: text}}, nil
		case 2:
			return []widget.RichTextSegment{&widget.TextSegment{Style: widget.RichTextStyleSubHeading, Text: text}}, nil
		default:
			seg := widget.TextSegment{Style: widget.RichTextStyleParagraph, Text: text}
			seg.Style.TextStyle.Bold = true
			return []widget.RichTextSegment{&seg}, nil
		}
	case *ast.ThematicBreak:
		return []widget.RichTextSegment{&widget.SeparatorSegment{}}, nil
	case *ast.Link:
		link, _ := url.Parse(string(t.Destination))
		text := decodeMarkdownText(forceIntoText(source, n))
		return []widget.RichTextSegment{&widget.HyperlinkSegment{Alignment: fyne.TextAlignLeading, Text: text, URL: link}}, nil
	case *ast.CodeSpan:
		return []widget.RichTextSegment{&widget.TextSegment{Style: widget.RichTextStyleCodeInline, Text: forceIntoText(source, n)}}, nil
	case *ast.CodeBlock, *ast.FencedCodeBlock:
		code := codeBlockText(source, n)
		if code == "" {
			return nil, nil
		}
		return []widget.RichTextSegment{&codeBlockSegment{Code: code}}, nil
	case *east.Strikethrough:
		text := decodeMarkdownText(forceIntoText(source, n))
		return []widget.RichTextSegment{&widget.TextSegment{Style: widget.RichTextStyleInline, Text: applyStrikethrough(text)}}, nil
	case *east.TaskCheckBox:
		glyph := "☐ "
		if t.IsChecked {
			glyph = "☑ "
		}
		return []widget.RichTextSegment{&widget.TextSegment{Style: widget.RichTextStyleInline, Text: glyph}}, nil
	case *ast.Emphasis:
		text := decodeMarkdownText(forceIntoText(source, n))
		if t.Level == 2 {
			return []widget.RichTextSegment{&widget.TextSegment{Style: widget.RichTextStyleStrong, Text: text}}, nil
		}
		return []widget.RichTextSegment{&widget.TextSegment{Style: widget.RichTextStyleEmphasis, Text: text}}, nil
	case *ast.Text:
		value := string(t.Value(source))
		if value == "" {
			return []widget.RichTextSegment{&widget.TextSegment{Style: widget.RichTextStyleInline, Text: " "}}, nil
		}
		value = suffixSpaceIfAppropriate(value, n)
		if blockquote {
			return []widget.RichTextSegment{&widget.TextSegment{Style: widget.RichTextStyleBlockquote, Text: value}}, nil
		}
		return []widget.RichTextSegment{&widget.TextSegment{Style: widget.RichTextStyleInline, Text: decodeMarkdownText(value)}}, nil
	case *ast.Blockquote:
		return renderMarkdownChildren(source, n, true, listDepth)
	case *east.Table:
		return []widget.RichTextSegment{buildTableSegment(source, t)}, nil
	}
	return nil, nil
}

func renderMarkdownChildren(source []byte, n ast.Node, blockquote bool, listDepth int) ([]widget.RichTextSegment, error) {
	children := make([]widget.RichTextSegment, 0, n.ChildCount())
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		segs, err := renderMarkdownNode(source, child, blockquote, listDepth)
		if err != nil {
			return children, err
		}
		children = append(children, segs...)
	}
	return children, nil
}

// applyStrikethrough returns text with a combining stroke after every rune so
// it renders struck-through despite Fyne having no strikethrough text style.
func applyStrikethrough(s string) string {
	var b strings.Builder
	b.Grow(len(s) * 2)
	for _, r := range s {
		b.WriteRune(r)
		b.WriteRune(strikeOverlay)
	}
	return b.String()
}

func codeBlockText(source []byte, n ast.Node) string {
	var data []byte
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		data = append(data, line.Value(source)...)
	}
	return strings.TrimSuffix(string(data), "\n")
}

func suffixSpaceIfAppropriate(text string, n ast.Node) string {
	next := n.NextSibling()
	if next != nil && next.Type() == ast.TypeInline && !strings.HasSuffix(text, " ") {
		return text + " "
	}
	return text
}

func forceIntoText(source []byte, n ast.Node) string {
	var b strings.Builder
	ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if t, ok := node.(*ast.Text); ok {
				b.Write(t.Value(source))
				b.WriteByte(' ')
			}
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSuffix(b.String(), " ")
}

func forceIntoHeadingText(source []byte, n ast.Node) string {
	var b strings.Builder
	ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if t, ok := node.(*ast.Text); ok {
				b.Write(t.Value(source))
			}
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

func decodeMarkdownText(text string) string {
	return html.UnescapeString(text)
}

// codeBlockSegment renders a fenced code block as monospace text on a bordered
// panel so it is visually separated from surrounding prose. It implements the
// public widget.RichTextSegment interface.
type codeBlockSegment struct {
	Code string
}

func (c *codeBlockSegment) Inline() bool { return false }

func (c *codeBlockSegment) Textual() string { return c.Code }

func (c *codeBlockSegment) Visual() fyne.CanvasObject {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameBackground))
	bg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	bg.StrokeWidth = 1
	bg.CornerRadius = theme.Size(theme.SizeNameInputRadius)

	label := widget.NewLabelWithStyle(c.Code, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	label.Wrapping = fyne.TextWrapOff

	return container.NewStack(bg, container.NewPadded(label))
}

func (c *codeBlockSegment) Update(fyne.CanvasObject) {}

func (c *codeBlockSegment) Select(_, _ fyne.Position) {}

func (c *codeBlockSegment) SelectedText() string { return c.Code }

func (c *codeBlockSegment) Unselect() {}

// tableGridGap is the thickness of the grid lines drawn between table cells.
const tableGridGap float32 = 1

// tableSegment renders a GFM table as a grid. Fyne's RichText has no concept of
// a table, so this custom segment lays cells out itself, drawing grid lines and
// honouring per-column alignment. Cell content is rendered with the same inline
// renderer as the rest of the document, so bold/code/links/strikethrough work
// inside cells.
type tableSegment struct {
	aligns []fyne.TextAlign
	header [][]widget.RichTextSegment
	rows   [][][]widget.RichTextSegment
}

func buildTableSegment(source []byte, tbl *east.Table) *tableSegment {
	t := &tableSegment{}
	for _, a := range tbl.Alignments {
		t.aligns = append(t.aligns, toFyneAlign(a))
	}
	for row := tbl.FirstChild(); row != nil; row = row.NextSibling() {
		var cells [][]widget.RichTextSegment
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			segs, _ := renderMarkdownChildren(source, cell, false, 0)
			cells = append(cells, segs)
		}
		if _, isHeader := row.(*east.TableHeader); isHeader {
			t.header = cells
		} else {
			t.rows = append(t.rows, cells)
		}
	}
	return t
}

func toFyneAlign(a east.Alignment) fyne.TextAlign {
	switch a {
	case east.AlignCenter:
		return fyne.TextAlignCenter
	case east.AlignRight:
		return fyne.TextAlignTrailing
	default:
		return fyne.TextAlignLeading
	}
}

func (t *tableSegment) Inline() bool { return false }

func (t *tableSegment) Textual() string {
	var b strings.Builder
	writeRow := func(cells [][]widget.RichTextSegment) {
		for i, cell := range cells {
			if i > 0 {
				b.WriteString("\t")
			}
			for _, s := range cell {
				b.WriteString(s.Textual())
			}
		}
		b.WriteString("\n")
	}
	if t.header != nil {
		writeRow(t.header)
	}
	for _, r := range t.rows {
		writeRow(r)
	}
	return b.String()
}

func (t *tableSegment) cols() int {
	cols := len(t.aligns)
	if len(t.header) > cols {
		cols = len(t.header)
	}
	for _, r := range t.rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	return cols
}

func (t *tableSegment) alignFor(col int) fyne.TextAlign {
	if col < len(t.aligns) {
		return t.aligns[col]
	}
	return fyne.TextAlignLeading
}

func (t *tableSegment) Visual() fyne.CanvasObject {
	cols := t.cols()
	if cols == 0 {
		return widget.NewRichText()
	}

	objects := make([]fyne.CanvasObject, 0, cols*(len(t.rows)+1))
	appendRow := func(cells [][]widget.RichTextSegment, header bool) {
		for c := 0; c < cols; c++ {
			var segs []widget.RichTextSegment
			if c < len(cells) {
				segs = cells[c]
			}
			objects = append(objects, newTableCell(segs, t.alignFor(c), header))
		}
	}
	if t.header != nil {
		appendRow(t.header, true)
	}
	for _, r := range t.rows {
		appendRow(r, false)
	}

	grid := container.New(&tableLayout{cols: cols, gap: tableGridGap}, objects...)
	border := canvas.NewRectangle(theme.Color(theme.ColorNameInputBorder))
	return container.NewStack(border, grid)
}

func (t *tableSegment) Update(fyne.CanvasObject) {}

func (t *tableSegment) Select(_, _ fyne.Position) {}

func (t *tableSegment) SelectedText() string { return t.Textual() }

func (t *tableSegment) Unselect() {}

// newTableCell builds a single cell: padded inline content over a fill so the
// grid-line border shows through the gaps left by tableLayout.
func newTableCell(segs []widget.RichTextSegment, align fyne.TextAlign, header bool) fyne.CanvasObject {
	fill := theme.Color(theme.ColorNameBackground)
	if header {
		fill = theme.Color(theme.ColorNameInputBackground)
	}
	bg := canvas.NewRectangle(fill)

	styled := make([]widget.RichTextSegment, 0, len(segs))
	for _, s := range segs {
		switch seg := s.(type) {
		case *widget.TextSegment:
			seg.Style.Alignment = align
			if header {
				seg.Style.TextStyle.Bold = true
			}
		case *widget.HyperlinkSegment:
			seg.Alignment = align
		}
		styled = append(styled, s)
	}
	if len(styled) == 0 {
		styled = append(styled, &widget.TextSegment{Style: widget.RichTextStyleInline, Text: " "})
	}

	rt := widget.NewRichText(styled...)
	rt.Wrapping = fyne.TextWrapOff
	return container.NewStack(bg, container.NewPadded(rt))
}

// tableLayout arranges cells row-major in a grid. Columns are sized to their
// widest cell, any slack width is shared evenly so the table fills the
// available width, and a gap is left around every cell so a background drawn
// behind the grid shows through as grid lines.
type tableLayout struct {
	cols int
	gap  float32
}

func (l *tableLayout) measure(objects []fyne.CanvasObject) (colWidths, rowHeights []float32) {
	rows := (len(objects) + l.cols - 1) / l.cols
	colWidths = make([]float32, l.cols)
	rowHeights = make([]float32, rows)
	for i, o := range objects {
		r, c := i/l.cols, i%l.cols
		m := o.MinSize()
		if m.Width > colWidths[c] {
			colWidths[c] = m.Width
		}
		if m.Height > rowHeights[r] {
			rowHeights[r] = m.Height
		}
	}
	return colWidths, rowHeights
}

func (l *tableLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	colWidths, rowHeights := l.measure(objects)
	w := l.gap
	for _, cw := range colWidths {
		w += cw + l.gap
	}
	h := l.gap
	for _, rh := range rowHeights {
		h += rh + l.gap
	}
	return fyne.NewSize(w, h)
}

func (l *tableLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	colWidths, rowHeights := l.measure(objects)

	minWidth := l.gap
	for _, cw := range colWidths {
		minWidth += cw + l.gap
	}
	if extra := size.Width - minWidth; extra > 0 && l.cols > 0 {
		share := extra / float32(l.cols)
		for c := range colWidths {
			colWidths[c] += share
		}
	}

	y := l.gap
	for r, rh := range rowHeights {
		x := l.gap
		for c := 0; c < l.cols; c++ {
			idx := r*l.cols + c
			if idx >= len(objects) {
				break
			}
			objects[idx].Move(fyne.NewPos(x, y))
			objects[idx].Resize(fyne.NewSize(colWidths[c], rh))
			x += colWidths[c] + l.gap
		}
		y += rh + l.gap
	}
}
