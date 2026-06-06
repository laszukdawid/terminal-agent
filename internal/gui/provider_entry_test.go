package gui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertSeg asserts a segment's text and whether it is the emphasized match
// (bold + accent color). Non-match segments must be plain.
func assertSeg(t *testing.T, s widget.RichTextSegment, wantText string, wantMatch bool) {
	t.Helper()
	ts, ok := s.(*widget.TextSegment)
	require.True(t, ok, "segment is not a *TextSegment")
	assert.Equal(t, wantText, ts.Text)
	assert.True(t, ts.Style.Inline, "segments must be inline to render on one line")
	if wantMatch {
		assert.True(t, ts.Style.TextStyle.Bold, "matched segment should be bold")
		assert.Equal(t, theme.ColorNamePrimary, ts.Style.ColorName, "matched segment should use the accent color")
	} else {
		assert.False(t, ts.Style.TextStyle.Bold, "plain segment should not be bold")
	}
}

func TestMatchSegments(t *testing.T) {
	// Match in the middle: google + "o" -> g | [o] | ogle
	segs := matchSegments("google", "o")
	require.Len(t, segs, 3)
	assertSeg(t, segs[0], "g", false)
	assertSeg(t, segs[1], "o", true)
	assertSeg(t, segs[2], "ogle", false)

	// Match at the start: openai + "o" -> [o] | penai (no leading plain segment)
	segs = matchSegments("openai", "O")
	require.Len(t, segs, 2)
	assertSeg(t, segs[0], "o", true)
	assertSeg(t, segs[1], "penai", false)

	// Empty query -> single plain segment with the whole text.
	segs = matchSegments("google", "")
	require.Len(t, segs, 1)
	assertSeg(t, segs[0], "google", false)

	// No match -> single plain segment.
	segs = matchSegments("mimo", "z")
	require.Len(t, segs, 1)
	assertSeg(t, segs[0], "mimo", false)

	// Multi-character match: bedrock + "ed" -> b | [ed] | rock
	segs = matchSegments("bedrock", "ed")
	require.Len(t, segs, 3)
	assertSeg(t, segs[0], "b", false)
	assertSeg(t, segs[1], "ed", true)
	assertSeg(t, segs[2], "rock", false)
}

func TestProviderEntryUpdatesTextAndNotifiesChanges(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	changes := []string{}
	entry := newProviderEntry("OpenAI", func(value string) {
		changes = append(changes, value)
	})

	entry.SetText("OpenA")
	entry.SetText("Anthropic")

	assert.Equal(t, "Anthropic", entry.Text)
	assert.Equal(t, []string{"OpenA", "Anthropic"}, changes)
}
