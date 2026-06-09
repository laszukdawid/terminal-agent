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
