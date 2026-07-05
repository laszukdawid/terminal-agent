package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flattenTextSegments collects every text-bearing leaf of a parsed segment
// tree, in order, so tests can assert on the visible preview content.
func flattenTextSegments(segs []widget.RichTextSegment) []*widget.TextSegment {
	var out []*widget.TextSegment
	for _, seg := range segs {
		switch s := seg.(type) {
		case *widget.TextSegment:
			out = append(out, s)
		case *widget.CodeBlockSegment:
			out = append(out, &widget.TextSegment{Style: widget.RichTextStyleCodeBlock, Text: s.Text})
		case *widget.ListSegment:
			out = append(out, flattenTextSegments(s.Items)...)
		case *widget.ParagraphSegment:
			out = append(out, flattenTextSegments(s.Texts)...)
		}
	}
	return out
}

func previewSegments(t *testing.T, content string, maxRunes, maxLines int) ([]*widget.TextSegment, string, bool) {
	t.Helper()
	rt := widget.NewRichText()
	rt.Wrapping = fyne.TextWrapWord
	truncated := renderResponseMarkdownPreview(rt, content, maxRunes, maxLines)
	segs := flattenTextSegments(rt.Segments)
	var joined strings.Builder
	for _, seg := range segs {
		joined.WriteString(seg.Text)
	}
	return segs, joined.String(), truncated
}

func TestRenderResponseMarkdownPreviewStylesSurviveTruncation(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	a.Settings().SetTheme(newBrandTheme())

	// The decorated prose prefix "$ aaaa " costs 7 runes, so a budget of 9
	// lands inside the styled word that follows it; the cut completes the word
	// and keeps its parsed style instead of leaking raw markers.
	tests := []struct {
		name      string
		content   string
		maxRunes  int
		maxLines  int
		wantWord  string
		wantStyle func(t *testing.T, seg *widget.TextSegment)
		leaked    []string
	}{
		{
			name:     "bold at the cut edge stays emphasized and whole",
			content:  "aaaa **Blob** tail",
			maxRunes: 9,
			maxLines: 8,
			wantWord: "Blob",
			wantStyle: func(t *testing.T, seg *widget.TextSegment) {
				assert.True(t, seg.Style.TextStyle.Bold, "cut word keeps the bold style")
			},
			leaked: []string{"**"},
		},
		{
			name:     "italic at the cut edge stays emphasized and whole",
			content:  "aaaa *Blob* tail",
			maxRunes: 9,
			maxLines: 8,
			wantWord: "Blob",
			wantStyle: func(t *testing.T, seg *widget.TextSegment) {
				assert.True(t, seg.Style.TextStyle.Italic, "cut word keeps the italic style")
			},
			leaked: []string{"*"},
		},
		{
			name:     "inline code at the cut edge stays code-styled and whole",
			content:  "aaaa `codetoken` tail",
			maxRunes: 9,
			maxLines: 8,
			wantWord: "codetoken",
			wantStyle: func(t *testing.T, seg *widget.TextSegment) {
				assert.True(t, seg.Style.TextStyle.Monospace, "cut word keeps the code style")
			},
			leaked: []string{"`"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			segs, joined, truncated := previewSegments(t, tc.content, tc.maxRunes, tc.maxLines)
			assert.True(t, truncated)
			for _, leak := range tc.leaked {
				assert.NotContains(t, joined, leak, "markdown syntax must never leak into the preview")
			}
			assert.Contains(t, joined, tc.wantWord+previewEllipsis, "the styled word is completed, then the preview ends")
			assert.NotContains(t, joined, "tail", "content past the cut is dropped")
			var styled *widget.TextSegment
			for _, seg := range segs {
				if strings.Contains(seg.Text, tc.wantWord) {
					styled = seg
				}
			}
			require.NotNil(t, styled)
			tc.wantStyle(t, styled)
		})
	}
}

func TestPreviewSourceCutsAtSafeBoundaries(t *testing.T) {
	const maxRunes = 100
	limit := maxRunes * previewSourceOverlapFactor
	tests := []struct {
		name    string
		content string
		want    string
		capped  bool
	}{
		{
			name:    "content within the overlap window is untouched",
			content: strings.Repeat("a", limit),
			want:    strings.Repeat("a", limit),
		},
		{
			name:    "blank line boundary ends every inline span",
			content: strings.Repeat("a", 150) + "\n\n" + strings.Repeat("b", 150),
			want:    strings.Repeat("a", 150),
			capped:  true,
		},
		{
			name:    "newline boundary keeps list items whole",
			content: strings.Repeat("a", 150) + "\n" + strings.Repeat("b", 150),
			want:    strings.Repeat("a", 150),
			capped:  true,
		},
		{
			name:    "space boundary keeps words whole",
			content: strings.Repeat("a", 150) + " " + strings.Repeat("b", 150),
			want:    strings.Repeat("a", 150),
			capped:  true,
		},
		{
			name:    "no boundary in the window falls back to a raw cut",
			content: strings.Repeat("a", 300),
			want:    strings.Repeat("a", limit),
			capped:  true,
		},
		{
			name:    "boundary below the display budget is ignored",
			content: strings.Repeat("a", 50) + " " + strings.Repeat("b", 300),
			want:    strings.Repeat("a", 50) + " " + strings.Repeat("b", limit-51),
			capped:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, capped := previewSource(tc.content, maxRunes)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.capped, capped)
		})
	}
}

func TestCompleteWord(t *testing.T) {
	longToken := strings.Repeat("x", previewWordOverrunMax+10)
	tests := []struct {
		name string
		text string
		cut  int
		want int
	}{
		{name: "mid-word extends to the word end", text: "some wordy tail", cut: 7, want: 10},
		{name: "cut on a space stays put", text: "some wordy tail", cut: 4, want: 4},
		{name: "cut after a space stays put", text: "some wordy tail", cut: 5, want: 5},
		{name: "word running to text end is completed", text: "some wordy", cut: 7, want: 10},
		{name: "token longer than the overrun is cut through", text: "ab " + longToken, cut: 5, want: 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, completeWord([]rune(tc.text), tc.cut))
		})
	}
}

func TestHistoryPreviewCompletesWords(t *testing.T) {
	value := strings.Repeat("a", historyPreviewMaxRunes-2) + " wordy tail"
	got, truncated := historyPreview(value, "")
	assert.True(t, truncated)
	assert.True(t, strings.HasSuffix(got, "wordy"+previewEllipsis), "got suffix: %q", got[len(got)-12:])
}

func TestRenderResponseMarkdownPreviewBounds(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	a.Settings().SetTheme(newBrandTheme())

	t.Run("short content is not truncated", func(t *testing.T) {
		_, joined, truncated := previewSegments(t, "Use **bold** and `code`.", historyPreviewMaxRunes, historyPreviewMaxLines)
		assert.False(t, truncated)
		assert.Contains(t, joined, "bold")
		assert.NotContains(t, joined, "**")
	})

	t.Run("code block is capped by the line budget without leaking fences", func(t *testing.T) {
		content := "intro\n\n```bash\nline1\nline2\nline3\nline4\nline5\n```\n\ntail"
		segs, joined, truncated := previewSegments(t, content, historyPreviewMaxRunes, 4)
		assert.True(t, truncated)
		assert.NotContains(t, joined, "```")
		assert.NotContains(t, joined, "line5")
		last := segs[len(segs)-1]
		assert.True(t, strings.HasSuffix(last.Text, previewEllipsis))
	})

	t.Run("display cut inside link text keeps the hyperlink", func(t *testing.T) {
		// The link source fits the parse window; the display budget then cuts
		// inside link text too long for word completion to finish.
		linkText := strings.Repeat("x", 100)
		content := "aa [" + linkText + "](https://example.com) tail"
		rt := widget.NewRichText()
		truncated := renderResponseMarkdownPreview(rt, content, 66, 8)
		assert.True(t, truncated)
		var link *widget.HyperlinkSegment
		for _, seg := range rt.Segments {
			if h, ok := seg.(*widget.HyperlinkSegment); ok {
				link = h
			}
		}
		require.NotNil(t, link, "the cut link must stay a hyperlink segment")
		assert.True(t, strings.HasSuffix(link.Text, previewEllipsis))
		assert.Less(t, len(link.Text), len(linkText))
		require.NotNil(t, link.URL)
		assert.Equal(t, "https://example.com", link.URL.String())
	})

	t.Run("link split by the source cut is dropped, not leaked", func(t *testing.T) {
		_, joined, truncated := previewSegments(t, "aaaa [click here for details](https://example.com)", 11, 8)
		assert.True(t, truncated)
		assert.NotContains(t, joined, "[", "a half-parsed link must not leak bracket syntax")
		assert.Contains(t, joined, "aaaa")
	})

	t.Run("emphasis split by the source cut is dropped, not leaked", func(t *testing.T) {
		content := strings.Repeat("c ", 60) + "**" + strings.Repeat("d", 200) + "** end"
		_, joined, truncated := previewSegments(t, content, 100, 8)
		assert.True(t, truncated)
		assert.NotContains(t, joined, "**", "a half-parsed emphasis must not leak its markers")
	})

	t.Run("source cut inside a fence stays code", func(t *testing.T) {
		content := "intro\n\n```bash\n" + strings.Repeat("echo x\n", 60)
		segs, joined, truncated := previewSegments(t, content, 100, 20)
		assert.True(t, truncated)
		assert.NotContains(t, joined, "```")
		require.NotEmpty(t, segs)
		assert.Contains(t, joined, "echo x")
	})

	t.Run("list is cut by the line budget", func(t *testing.T) {
		content := "- one\n- two\n- three\n- four\n- five\n- six\n- seven\n- eight\n- nine\n- ten"
		_, joined, truncated := previewSegments(t, content, historyPreviewMaxRunes, 4)
		assert.True(t, truncated)
		assert.Contains(t, joined, "one")
		assert.NotContains(t, joined, "ten")
	})

	t.Run("exact fit is not reported as truncation", func(t *testing.T) {
		// "$ abcd" decorates to exactly 6 runes of visible text.
		_, joined, truncated := previewSegments(t, "abcd", 6, 8)
		assert.False(t, truncated)
		assert.Contains(t, joined, "abcd")
		assert.NotContains(t, joined, previewEllipsis)
	})

	t.Run("oversized source is capped and reported truncated", func(t *testing.T) {
		content := strings.Repeat("word ", historyPreviewMaxRunes*previewSourceOverlapFactor)
		_, joined, truncated := previewSegments(t, content, historyPreviewMaxRunes, historyPreviewMaxLines)
		assert.True(t, truncated)
		assert.True(t, strings.HasSuffix(joined, previewEllipsis), "a source-capped preview still reads as cut")
	})
}

func TestHistoryQueryLineMarksInput(t *testing.T) {
	rt := historyQueryLine("list files")
	require.Len(t, rt.Segments, 2)

	marker, ok := rt.Segments[0].(*widget.TextSegment)
	require.True(t, ok)
	assert.Equal(t, inputPrompt+" ", marker.Text)
	assert.Equal(t, theme.ColorNamePrimary, marker.Style.ColorName)
	assert.True(t, marker.Style.TextStyle.Bold)

	query, ok := rt.Segments[1].(*widget.TextSegment)
	require.True(t, ok)
	assert.Equal(t, "list files", query.Text)
	assert.True(t, query.Style.TextStyle.Bold)
}
