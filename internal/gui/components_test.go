package gui

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"

	"github.com/laszukdawid/terminal-agent/internal/sessionlog"
	"github.com/laszukdawid/terminal-agent/internal/taskmemory"
)

func TestElideToWidth(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	a.Settings().SetTheme(newBrandTheme())

	style := fyne.TextStyle{Monospace: true}
	const size float32 = 14
	full := "bedrock / eu.anthropic.claude-sonnet-4-5-20250929-v1:0 | ~/projects/terminal-agent | 1m32s"
	fullWidth := fyne.MeasureText(full, size, style).Width
	ellipsisWidth := fyne.MeasureText(elidedTextEllipsis, size, style).Width

	tests := []struct {
		name     string
		width    float32
		wantFull bool
	}{
		{name: "full text fits untouched", width: fullWidth, wantFull: true},
		{name: "narrower width elides", width: fullWidth / 2},
		{name: "tiny width still renders the ellipsis", width: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := elideToWidth(full, tc.width, size, style)
			if tc.wantFull {
				assert.Equal(t, full, got)
				return
			}
			assert.True(t, strings.HasSuffix(got, elidedTextEllipsis))
			assert.Less(t, len(got), len(full))
			if tc.width >= ellipsisWidth {
				assert.LessOrEqual(t, fyne.MeasureText(got, size, style).Width, tc.width)
			}
		})
	}
}

func TestElidedTextMinWidthIndependentOfContent(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	a.Settings().SetTheme(newBrandTheme())

	full := strings.Repeat("wide ", 60)
	et := newElidedText(full, brandSecondaryText)
	test.WidgetRenderer(et)

	minSize := et.MinSize()
	assert.Less(t, minSize.Width, float32(30), "min width must not scale with content")
	assert.Positive(t, minSize.Height)

	et.Resize(fyne.NewSize(200, minSize.Height))
	assert.True(t, strings.HasSuffix(et.text.Text, elidedTextEllipsis), "drawn text is elided to the given width")
	assert.Equal(t, full, et.Text(), "full string stays available")
}

// Switching to a browse tab must not raise the window's minimum width: card
// title/meta lines elide instead of demanding their full rendered width, which
// previously propagated through the vertical scroll and widened the whole
// window (see the F.A.Q. in docs/developers.md).
func TestBrowseTabsDoNotWidenWindow(t *testing.T) {
	longCwd := "/Users/someone/very/long/path/" + strings.Repeat("nested/", 12) + "project"
	longModel := "eu.anthropic.claude-sonnet-4-5-20250929-v1:0"

	t.Run("history", func(t *testing.T) {
		a := test.NewApp()
		defer a.Quit()
		g := NewApp(&recordingService{}, voiceGUIConfig{}, AppOptions{FyneApp: a})
		defer g.Quit()
		before := g.popup.window.Content().MinSize().Width

		run := sessionlog.Summary{
			Kind:        "task",
			Provider:    "bedrock",
			Model:       longModel,
			Cwd:         longCwd,
			Request:     strings.Repeat("list docker containers ", 20),
			Response:    "Fast way\n\n```bash\n" + strings.Repeat("averylongunbreakabletoken", 10) + "\n```\n\n" + strings.Repeat("output ", 80),
			CreatedAt:   time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
			CompletedAt: time.Date(2026, 7, 1, 12, 1, 32, 0, time.UTC),
		}
		g.popup.setHistory([]sessionlog.Summary{run}, "")
		g.popup.inputGroup.Hide()
		g.popup.responseSection.Hide()
		g.popup.historySection.Show()

		assert.LessOrEqual(t, g.popup.window.Content().MinSize().Width, before)
	})

	t.Run("task memory", func(t *testing.T) {
		seedTaskMemoryDB(t, []taskmemory.Record{{
			Kind:           taskmemory.KindToolSuccess,
			OS:             runtime.GOOS,
			Arch:           runtime.GOARCH,
			ToolName:       "unix",
			CWD:            longCwd,
			SearchableText: "Kind: tool_success\nUser request: list docker containers\nTool: unix\nInput: docker ps",
			CreatedAt:      time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
		}})
		a := test.NewApp()
		defer a.Quit()
		g := NewApp(&recordingService{}, voiceGUIConfig{}, AppOptions{FyneApp: a})
		defer g.Quit()
		before := g.popup.window.Content().MinSize().Width

		g.setMode(guiModeMemory)

		assert.LessOrEqual(t, g.popup.window.Content().MinSize().Width, before)
	})
}
