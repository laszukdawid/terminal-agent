package gui

import (
	"strings"
	"unicode"

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

// previewEllipsis marks preview text cut short by a rune or line budget.
const previewEllipsis = "..."

// renderResponseMarkdownPreview renders content through the same pipeline as
// renderResponseMarkdown but bounds the result for card previews. Only a
// bounded overlap window of the source is parsed (see previewSource), and the
// display truncation then happens on the parsed segment tree rather than the
// source markdown, so a cut can never split inline syntax: a preview ending
// inside **bold** or `code` keeps that styling on the visible prefix instead
// of leaking raw markers like "**Bl". It reports whether anything was cut.
func renderResponseMarkdownPreview(rt *widget.RichText, content string, maxRunes, maxLines int) bool {
	content, sourceCapped := previewSource(unwrapMarkdownFence(content), maxRunes)
	rt.ParseMarkdown(decorateDollarMarkers(content))
	budget := &richTextBudget{runes: maxRunes, lines: maxLines}
	segs, truncated := truncateRichTextSegments(colorizeDollarMarkers(rt.Segments), budget)
	if sourceCapped && !truncated {
		// The display budget was not exhausted but source was dropped: still end
		// the preview with an ellipsis so it reads as cut.
		segs = append(segs, &widget.TextSegment{Style: widget.RichTextStyleInline, Text: previewEllipsis})
	}
	rt.Segments = spaceResponseBlocks(segs)
	rt.Refresh()
	return truncated || sourceCapped
}

// previewSourceOverlapFactor is how much more markdown source than the display
// rune budget a preview parses. The margin absorbs markup overhead (emphasis
// markers, link URLs, fence lines) that costs source runes but renders small,
// and gives the word-completing display cut room to finish its word.
const previewSourceOverlapFactor = 2

// previewWordOverrunMax is how far past a rune budget a preview may reach to
// finish the word it would otherwise cut through. Tokens longer than this
// (URLs, hashes) are cut mid-token after all.
const previewWordOverrunMax = 24

// previewSource bounds how much markdown source a preview parses, so a card
// never pays for parsing a whole long session response. The cut lands on the
// safest boundary available inside the overlap window: a blank line ends every
// inline span (emphasis, code spans, and links cannot cross a block boundary),
// a newline keeps list items whole, and a space keeps words whole. Cutting on
// such a boundary makes closing split syntax unnecessary; the only construct
// that can straddle these cuts is a code fence, which markdown already treats
// as code through end-of-input when unterminated.
func previewSource(content string, maxRunes int) (string, bool) {
	runes := []rune(content)
	limit := maxRunes * previewSourceOverlapFactor
	if len(runes) <= limit {
		return content, false
	}
	window := string(runes[:limit])
	// Prefer not to cut below the display budget, or the preview would starve.
	floor := len(string(runes[:maxRunes]))
	if i := strings.LastIndex(window, "\n\n"); i >= floor {
		window = window[:i]
	} else if i := strings.LastIndexByte(window, '\n'); i >= floor {
		window = window[:i]
	} else if i := strings.LastIndexFunc(window, unicode.IsSpace); i >= floor {
		window = window[:i]
	}
	return trimSplitInlineSyntax(window), true
}

// trimSplitInlineSyntax guards the source cut of previewSource: a cut that
// lands after an unclosed inline construct (emphasis, strikethrough, code
// span, or link) would make the parser render its markers literally, leaking
// "**Bl" or "[text" into the preview. Appending closers is not an option — a
// half-cut URL cannot be repaired and invented closers would style text the
// author never closed — so the trailing paragraph is instead trimmed back to
// the last boundary where everything was closed. A window ending inside a
// fenced code block is returned unchanged: markdown treats an unterminated
// fence as code through end-of-input, where inline markers are literal anyway.
func trimSplitInlineSyntax(window string) string {
	tailStart, inFence := lastInlineRegionStart(window)
	if inFence {
		return window
	}
	tail := []rune(window[tailStart:])

	var boldOpen, italicOpen, strikeOpen bool
	codeRun := 0 // length of the backtick run that opened a code span
	const (
		linkNone = iota
		linkText // after "["
		linkURL  // after "]("
	)
	linkPhase := linkNone
	allClosed := func() bool {
		return codeRun == 0 && !boldOpen && !italicOpen && !strikeOpen && linkPhase == linkNone
	}

	lastSafe := 0
	for i := 0; i < len(tail); {
		r := tail[i]
		switch {
		case r == '`':
			run := runeRunLength(tail, i, '`')
			if codeRun == 0 {
				codeRun = run
			} else if run == codeRun {
				codeRun = 0
			}
			i += run
			continue
		case codeRun > 0:
			// inside a code span every other marker is literal
		case r == '*':
			run := runeRunLength(tail, i, '*')
			for n := run; n > 0; n -= 2 {
				if n >= 2 {
					boldOpen = !boldOpen
				} else {
					italicOpen = !italicOpen
				}
			}
			i += run
			continue
		case r == '~':
			run := runeRunLength(tail, i, '~')
			if run >= 2 {
				strikeOpen = !strikeOpen
			}
			i += run
			continue
		case r == '[' && linkPhase == linkNone:
			linkPhase = linkText
		case r == ']' && linkPhase == linkText:
			if i+1 < len(tail) && tail[i+1] == '(' {
				linkPhase = linkURL
				i += 2
				continue
			}
			linkPhase = linkNone
		case r == ')' && linkPhase == linkURL:
			linkPhase = linkNone
		case unicode.IsSpace(r) && allClosed():
			lastSafe = i
		}
		i++
	}
	if allClosed() {
		return window
	}
	return strings.TrimRight(window[:tailStart+len(string(tail[:lastSafe]))], " \n")
}

// lastInlineRegionStart finds where the trailing inline-scan region of a
// window begins: after the last blank line or closed code fence, both of which
// terminate every inline construct. It also reports whether the window ends
// inside a fenced code block.
func lastInlineRegionStart(window string) (int, bool) {
	inFence := false
	start := 0
	offset := 0
	for _, line := range strings.SplitAfter(window, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~"):
			inFence = !inFence
			if !inFence {
				start = offset + len(line)
			}
		case !inFence && trimmed == "":
			start = offset + len(line)
		}
		offset += len(line)
	}
	return start, inFence
}

func runeRunLength(runes []rune, i int, r rune) int {
	n := 0
	for i+n < len(runes) && runes[i+n] == r {
		n++
	}
	return n
}

// completeWord extends a cut index that lands inside a word to that word's
// end, up to previewWordOverrunMax extra runes, so previews prefer "word..."
// over "wo...". A cut already on a boundary, or inside a token too long to
// finish, is returned unchanged.
func completeWord(runes []rune, cut int) int {
	if cut <= 0 || cut >= len(runes) || unicode.IsSpace(runes[cut-1]) || unicode.IsSpace(runes[cut]) {
		return cut
	}
	limit := min(len(runes), cut+previewWordOverrunMax)
	for i := cut; i < limit; i++ {
		if unicode.IsSpace(runes[i]) {
			return i
		}
	}
	if limit == len(runes) {
		return limit // the word runs to the end of the text
	}
	return cut
}

// richTextBudget tracks the remaining rune and line allowance while a parsed
// segment tree is truncated for a preview.
type richTextBudget struct {
	runes int
	lines int
}

func (b *richTextBudget) exhausted() bool { return b.runes <= 0 || b.lines <= 0 }

// take consumes text against the budget, returning the kept prefix and whether
// anything was cut. Newlines inside the text (code blocks) count against the
// line budget so a tall block cannot blow up a preview's height.
func (b *richTextBudget) take(text string) (string, bool) {
	runes := []rune(text)
	cut := false
	if len(runes) > b.runes {
		if keep := completeWord(runes, b.runes); keep < len(runes) {
			runes = runes[:keep]
			cut = true
		}
	}
	newlines := 0
	for i, r := range runes {
		if r != '\n' {
			continue
		}
		newlines++
		if newlines >= b.lines {
			runes = runes[:i]
			cut = true
			break
		}
	}
	kept := string(runes)
	b.runes -= len(runes)
	b.lines -= strings.Count(kept, "\n")
	return kept, cut
}

// truncateRichTextSegments walks a parsed segment tree and keeps segments until
// the budget runs out. A segment cut mid-text is cloned with its style intact
// and the ellipsis appended, so emphasis, inline code, code blocks, and links
// survive truncation; every kept segment keeps rendering exactly as parsed.
func truncateRichTextSegments(segs []widget.RichTextSegment, budget *richTextBudget) ([]widget.RichTextSegment, bool) {
	out := make([]widget.RichTextSegment, 0, len(segs))
	for i, seg := range segs {
		if budget.exhausted() {
			if !segmentsHaveVisibleContent(segs[i:]) {
				return out, false // only terminators remain; an exact fit is not a cut
			}
			return append(out, &widget.TextSegment{Style: widget.RichTextStyleInline, Text: previewEllipsis}), true
		}
		switch s := seg.(type) {
		case *widget.TextSegment:
			text, cut := budget.take(s.Text)
			if cut {
				clone := *s
				clone.Text = text + previewEllipsis
				return append(out, &clone), true
			}
			out = append(out, s)
		case *widget.HyperlinkSegment:
			text, cut := budget.take(s.Text)
			if cut {
				clone := *s
				clone.Text = text + previewEllipsis
				return append(out, &clone), true
			}
			out = append(out, s)
		case *widget.ListSegment:
			items, cut := truncateRichTextSegments(s.Items, budget)
			if cut {
				return append(out, &widget.ListSegment{Items: items, Ordered: s.Ordered}), true
			}
			out = append(out, s)
		case *widget.ParagraphSegment:
			texts, cut := truncateRichTextSegments(s.Texts, budget)
			if cut {
				return append(out, &widget.ParagraphSegment{Texts: texts}), true
			}
			out = append(out, s)
		case *widget.CodeBlockSegment:
			text, cut := budget.take(s.Text)
			if cut {
				clone := *s
				clone.Text = text + previewEllipsis
				return append(out, &clone), true
			}
			out = append(out, s)
		default:
			// Opaque segments (tables, checkboxes, separators) are kept or dropped
			// whole: their textual size is charged against the budget, and one that
			// no longer fits ends the preview rather than rendering partially.
			textual := []rune(seg.Textual())
			if len(textual) > budget.runes {
				return append(out, &widget.TextSegment{Style: widget.RichTextStyleInline, Text: previewEllipsis}), true
			}
			budget.runes -= len(textual)
			out = append(out, seg)
		}
		if !seg.Inline() {
			budget.lines--
		}
	}
	return out, false
}

// segmentsHaveVisibleContent reports whether any segment would draw something:
// blank paragraph terminators and separators do not count, so an exhausted
// budget with only those left is not reported as a truncation.
func segmentsHaveVisibleContent(segs []widget.RichTextSegment) bool {
	for _, seg := range segs {
		switch s := seg.(type) {
		case *widget.ListSegment:
			if segmentsHaveVisibleContent(s.Items) {
				return true
			}
		case *widget.ParagraphSegment:
			if segmentsHaveVisibleContent(s.Texts) {
				return true
			}
		case *widget.ImageSegment:
			return true
		default:
			if strings.TrimSpace(seg.Textual()) != "" {
				return true
			}
		}
	}
	return false
}
