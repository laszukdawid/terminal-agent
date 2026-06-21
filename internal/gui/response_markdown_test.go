package gui

import (
	"testing"

	"fyne.io/fyne/v2/widget"
)

// isBlankSpacer reports whether a segment is one of the empty, non-inline
// paragraph rows that spaceResponseBlocks inserts for vertical spacing.
func isBlankSpacer(seg widget.RichTextSegment) bool {
	ts, ok := seg.(*widget.TextSegment)
	return ok && ts.Text == "" && !ts.Style.Inline
}

func parseResponseSegments(md string) []widget.RichTextSegment {
	return widget.NewRichTextFromMarkdown(md).Segments
}

// standaloneBlankRowsAfterBullet mirrors Fyne's row-break rule for the text run
// of a list item's leaf paragraph: a non-inline segment closes the open row (a
// merge) the first time, and only renders as its own blank row when no row is
// open. A list item's bullet is laid out before the leaf paragraph, so a row is
// open on entry. This lets a test assert the rendered spacing ("how many blank
// rows") rather than the raw segment count, which is the real contract.
func standaloneBlankRowsAfterBullet(texts []widget.RichTextSegment) int {
	rowOpen := true // the item's bullet opens the row before its text is laid out
	blanks := 0
	for _, seg := range texts {
		if seg.Inline() {
			rowOpen = true
			continue
		}
		if rowOpen {
			rowOpen = false // terminator closes the open row (merged, no blank)
			continue
		}
		blanks++ // standalone terminator renders as a blank row
	}
	return blanks
}

func TestSpaceResponseBlocksTopLevel(t *testing.T) {
	tests := []struct {
		name           string
		markdown       string
		wantSpacerAfop bool // expect a blank spacer immediately after the first list
	}{
		{
			name:           "bulleted list returning to prose",
			markdown:       "- one\n- two\n\nthen some prose\n",
			wantSpacerAfop: true,
		},
		{
			name:           "ordered list returning to prose",
			markdown:       "1. one\n2. two\n\nthen some prose\n",
			wantSpacerAfop: true,
		},
		{
			name:           "list at the end of the response",
			markdown:       "intro prose\n\n- one\n- two\n",
			wantSpacerAfop: false,
		},
		{
			name:           "list returning to a heading",
			markdown:       "- one\n- two\n\n# A heading\n",
			wantSpacerAfop: true,
		},
		{
			name:           "prose only has no list to space",
			markdown:       "just a single paragraph of prose\n",
			wantSpacerAfop: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := spaceResponseBlocks(parseResponseSegments(tt.markdown))

			spacerAfterList := false
			for i, seg := range got {
				if _, ok := seg.(*widget.ListSegment); !ok {
					continue
				}
				if i+1 < len(got) && isBlankSpacer(got[i+1]) {
					spacerAfterList = true
				}
				break
			}
			if spacerAfterList != tt.wantSpacerAfop {
				t.Fatalf("spacer after first list = %v, want %v", spacerAfterList, tt.wantSpacerAfop)
			}
		})
	}
}

func TestSpaceResponseBlocksPreservesContent(t *testing.T) {
	// Spacing must only interleave blank rows; every original top-level segment
	// must still appear, in its original order. (Paragraph terminators are also
	// empty non-inline segments, so identity, not emptiness, is the reliable
	// signal here.)
	orig := parseResponseSegments("- one\n- two\n\nthen prose\n")
	after := spaceResponseBlocks(orig)

	if len(after) <= len(orig) {
		t.Fatalf("expected spacing to add segments: orig=%d after=%d", len(orig), len(after))
	}
	matched := 0
	for _, seg := range after {
		if matched < len(orig) && seg == orig[matched] {
			matched++
		}
	}
	if matched != len(orig) {
		t.Fatalf("original segments not preserved in order: matched %d of %d", matched, len(orig))
	}
}

func TestSpaceNestedListReturns(t *testing.T) {
	// A nested sub-list followed by a sibling at the parent tier should render
	// exactly one blank row after the sub-list, regardless of whether the list is
	// tight, loose, or has multi-paragraph items (which differ in how the parsed
	// item paragraph already terminates).
	tests := []struct {
		name     string
		markdown string
	}{
		{
			name:     "tight nested list",
			markdown: "- top a\n  - nested a1\n  - nested a2\n- top b\n",
		},
		{
			name:     "loose nested list",
			markdown: "- top a\n\n  - nested a1\n\n  - nested a2\n\n- top b\n",
		},
		{
			name:     "multi-paragraph nested item",
			markdown: "- top a\n  - nested a1\n\n    second paragraph of a1\n- top b\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			segs := spaceResponseBlocks(parseResponseSegments(tt.markdown))
			top, ok := segs[0].(*widget.ListSegment)
			if !ok {
				t.Fatalf("expected first segment to be a list, got %T", segs[0])
			}
			nested, ok := top.Items[1].(*widget.ListSegment)
			if !ok {
				t.Fatalf("expected Items[1] to be a nested list, got %T", top.Items[1])
			}
			lastItem, ok := nested.Items[len(nested.Items)-1].(*widget.ParagraphSegment)
			if !ok {
				t.Fatalf("expected last nested item to be a paragraph, got %T", nested.Items[len(nested.Items)-1])
			}
			if rows := standaloneBlankRowsAfterBullet(lastItem.Texts); rows != 1 {
				t.Fatalf("expected exactly one blank row after the nested list, got %d", rows)
			}
		})
	}
}

func TestSpaceNestedListNotIdempotent(t *testing.T) {
	// appendBlankAfterList documents that it expects a freshly parsed tree.
	// Applying it twice accumulates spacing; this test pins that precondition so a
	// future change that reuses parsed segments is forced to reconsider it.
	md := "- top a\n  - nested a1\n  - nested a2\n- top b\n"
	segs := parseResponseSegments(md)
	spaceResponseBlocks(segs)
	once := standaloneBlankRowsAfterBullet(lastNestedLeaf(t, segs).Texts)
	spaceResponseBlocks(segs)
	twice := standaloneBlankRowsAfterBullet(lastNestedLeaf(t, segs).Texts)
	if once != 1 {
		t.Fatalf("expected one blank row after a single pass, got %d", once)
	}
	if twice <= once {
		t.Fatalf("expected re-application to accumulate spacing (precondition), got once=%d twice=%d", once, twice)
	}
}

func lastNestedLeaf(t *testing.T, segs []widget.RichTextSegment) *widget.ParagraphSegment {
	t.Helper()
	top := segs[0].(*widget.ListSegment)
	nested := top.Items[1].(*widget.ListSegment)
	return nested.Items[len(nested.Items)-1].(*widget.ParagraphSegment)
}

func TestSpaceNestedListNoTrailingSpacerWhenLast(t *testing.T) {
	// A nested sub-list that is the final item of its parent must NOT gain a
	// trailing blank; that transition is handled one tier up.
	md := "- top a\n- top b\n  - nested b1\n  - nested b2\n"
	segs := spaceResponseBlocks(parseResponseSegments(md))

	top := segs[0].(*widget.ListSegment)
	nested := top.Items[len(top.Items)-1].(*widget.ListSegment)
	lastItem := nested.Items[len(nested.Items)-1].(*widget.ParagraphSegment)
	for _, seg := range lastItem.Texts {
		if isBlankSpacer(seg) {
			t.Fatalf("unexpected trailing spacer on a terminal nested list")
		}
	}
}
