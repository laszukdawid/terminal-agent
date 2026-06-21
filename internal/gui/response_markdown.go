package gui

import (
	"strings"

	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// unwrapMarkdownFence strips an outer ```markdown or ```md code fence so
// the inner content is parsed as rich text instead of a single code block.
// LLMs commonly wrap markdown responses this way.
func unwrapMarkdownFence(content string) string {
	trimmed := strings.TrimSpace(content)
	lower := strings.ToLower(trimmed)

	var prefixLen int
	switch {
	case strings.HasPrefix(lower, "```markdown\n"):
		prefixLen = len("```markdown\n")
	case strings.HasPrefix(lower, "```md\n"):
		prefixLen = len("```md\n")
	default:
		return content
	}

	inner := trimmed[prefixLen:]
	if !strings.HasSuffix(inner, "\n```") {
		return content
	}
	return inner[:len(inner)-4]
}

// decorateDollarMarkers prefixes prose paragraphs with the terminal "$" output
// marker from the brand response styling. It deliberately skips lines that
// carry markdown block syntax (headings, lists, quotes, tables, code fences,
// and indented code) so the marker never corrupts structured output; those
// blocks render normally. Continuation lines inside a paragraph are left as-is
// so they indent under the marked first line rather than repeating "$".
func decorateDollarMarkers(content string) string {
	lines := strings.Split(content, "\n")
	inFence := false
	atParagraphStart := true
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			atParagraphStart = false
			continue
		}
		if inFence {
			continue
		}
		if trimmed == "" {
			atParagraphStart = true
			continue
		}
		if atParagraphStart && isProseLine(line) {
			lines[i] = "$ " + line
		}
		atParagraphStart = false
	}
	return strings.Join(lines, "\n")
}

// renderResponseMarkdown parses a markdown response into the given RichText and
// applies the shared brand post-processing: the "$" prose markers and the
// inter-block spacing that keeps dense replies readable. Both the ask response
// panel and the task transcript render through here so the two surfaces stay in
// sync.
func renderResponseMarkdown(rt *widget.RichText, content string) {
	rt.ParseMarkdown(decorateDollarMarkers(unwrapMarkdownFence(content)))
	rt.Segments = spaceResponseBlocks(colorizeDollarMarkers(rt.Segments))
	rt.Refresh()
}

// spaceResponseBlocks adds vertical breathing room between distinct blocks of a
// parsed markdown response. Fyne's RichText layout separates every block-level
// row by the same single line of spacing, which reads as dense when structured
// answers mix lists with prose. This inserts a blank spacer row at the
// transitions that benefit most from separation:
//
//   - a list (bulleted or numbered) returning to prose: the paragraph or
//     heading that follows the list,
//   - a nested sub-list returning to a higher outline tier: the next top-tier
//     item after the sub-list ends.
//
// Spacing between sibling items at the same tier, and between the wrapped lines
// of a single paragraph, is intentionally left untouched so only the structural
// boundaries gain space.
func spaceResponseBlocks(segs []widget.RichTextSegment) []widget.RichTextSegment {
	for _, seg := range segs {
		if list, ok := seg.(*widget.ListSegment); ok {
			spaceNestedListReturns(list)
		}
	}

	out := make([]widget.RichTextSegment, 0, len(segs)+2)
	for i, seg := range segs {
		out = append(out, seg)
		if _, isList := seg.(*widget.ListSegment); !isList {
			continue
		}
		if i+1 < len(segs) && beginsProseBlock(segs[i+1]) {
			out = append(out, blankResponseLine())
		}
	}
	return out
}

// spaceNestedListReturns walks a list and, wherever a nested sub-list is
// followed by an item at the parent tier, adds a blank row after the sub-list so
// the return to the higher tier is visually separated. It recurses so deeper
// nesting is handled first.
func spaceNestedListReturns(list *widget.ListSegment) {
	for i, item := range list.Items {
		sub, ok := item.(*widget.ListSegment)
		if !ok {
			continue
		}
		spaceNestedListReturns(sub)
		if i+1 >= len(list.Items) {
			continue // the sub-list ends the list; the trailing transition is handled one tier up
		}
		if _, nextIsList := list.Items[i+1].(*widget.ListSegment); !nextIsList {
			appendBlankAfterList(sub)
		}
	}
}

// appendBlankAfterList makes a nested list render exactly one blank row after
// its last item by appending empty paragraph segments to the last leaf item.
// An empty paragraph only renders as a blank row when it is laid out after a row
// break, so the number needed depends on how the item already ends:
//
//   - a tight item ends on inline text (its row is still open), so it needs a
//     terminator to close that row followed by the standalone blank row,
//   - a loose or multi-paragraph item already ends on a non-inline terminator,
//     so a single blank row segment is enough.
//
// This expects a freshly parsed segment tree; it is not idempotent, because an
// already-appended blank is indistinguishable from a loose item's own
// terminator. renderResponseMarkdown re-parses on every call, which satisfies
// that precondition.
func appendBlankAfterList(list *widget.ListSegment) {
	if len(list.Items) == 0 {
		return
	}
	last := list.Items[len(list.Items)-1]
	if sub, ok := last.(*widget.ListSegment); ok {
		appendBlankAfterList(sub)
		return
	}
	para, ok := last.(*widget.ParagraphSegment)
	if !ok {
		return
	}
	if endsInline(para.Texts) {
		para.Texts = append(para.Texts, blankResponseLine())
	}
	para.Texts = append(para.Texts, blankResponseLine())
}

// endsInline reports whether the final segment of a paragraph's contents leaves
// a row open (inline) rather than closing it with a block terminator.
func endsInline(texts []widget.RichTextSegment) bool {
	if len(texts) == 0 {
		return false
	}
	return texts[len(texts)-1].Inline()
}

// beginsProseBlock reports whether a segment starts a paragraph or heading, as
// opposed to continuing or terminating a block. Prose blocks open with an inline
// text or hyperlink segment; the trailing paragraph terminator is non-inline.
func beginsProseBlock(seg widget.RichTextSegment) bool {
	switch s := seg.(type) {
	case *widget.TextSegment:
		return s.Inline()
	case *widget.HyperlinkSegment:
		return true
	}
	return false
}

// blankResponseLine returns an empty, non-inline paragraph segment. Laid out on
// its own it renders as a single blank row, which is the spacing unit used to
// separate response blocks.
func blankResponseLine() widget.RichTextSegment {
	return &widget.TextSegment{Style: widget.RichTextStyleParagraph}
}

// colorizeDollarMarkers recolors the leading "$" output marker of each marked
// prose segment to the brand accent green, matching the terminal response
// styling in the mock while leaving the body text in the default color.
func colorizeDollarMarkers(segs []widget.RichTextSegment) []widget.RichTextSegment {
	out := make([]widget.RichTextSegment, 0, len(segs)+4)
	for _, seg := range segs {
		ts, ok := seg.(*widget.TextSegment)
		if !ok || !strings.HasPrefix(ts.Text, "$ ") {
			out = append(out, seg)
			continue
		}
		marker := &widget.TextSegment{Text: "$", Style: ts.Style}
		marker.Style.ColorName = theme.ColorNamePrimary
		rest := &widget.TextSegment{Text: ts.Text[1:], Style: ts.Style}
		out = append(out, marker, rest)
	}
	return out
}

// isProseLine reports whether a line is plain prose that should receive a "$"
// marker, as opposed to a markdown block element that must be left untouched.
func isProseLine(line string) bool {
	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
		return false // indented code / nested block
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	switch trimmed[0] {
	case '#', '-', '*', '+', '>', '|', '`', '=':
		return false
	}
	// Ordered list item: "1." / "2)" etc.
	if trimmed[0] >= '0' && trimmed[0] <= '9' {
		rest := strings.TrimLeft(trimmed, "0123456789")
		if strings.HasPrefix(rest, ".") || strings.HasPrefix(rest, ")") {
			return false
		}
	}
	return true
}
