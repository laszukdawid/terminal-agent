package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func TestBuildOutputWrapsGeneralResponseText(t *testing.T) {
	p := &popupWindow{}

	p.buildOutput()

	if p.outputScroll.Direction != container.ScrollVerticalOnly {
		t.Fatalf("outputScroll.Direction = %v, want %v", p.outputScroll.Direction, container.ScrollVerticalOnly)
	}
	if p.outputField.Wrapping != fyne.TextWrapWord {
		t.Fatalf("outputField.Wrapping = %v, want %v", p.outputField.Wrapping, fyne.TextWrapWord)
	}
	if p.transcriptBody == nil {
		t.Fatal("buildOutput should initialize the transcript container")
	}
	if p.errorLabel.Wrapping != fyne.TextWrapBreak {
		t.Fatalf("errorLabel.Wrapping = %v, want %v", p.errorLabel.Wrapping, fyne.TextWrapBreak)
	}
}

func TestToolOutputScrollsHorizontallyInsteadOfWideningWindow(t *testing.T) {
	test.NewApp()

	p := &popupWindow{}
	p.buildOutput()

	// A single very long, unbreakable line (like the long paths in real tool
	// output) that would otherwise force the whole window wider.
	longLine := strings.Repeat("/System/Library/PrivateFrameworks/Accessory.framework/", 8)
	p.setTranscript([]transcriptBlock{
		{Kind: transcriptBlockToolOutput, Chunks: []string{longLine + "\n"}},
	})

	grid := p.transcriptViews[0].textGrid
	if grid == nil {
		t.Fatal("expected a tool-output text grid")
	}
	if grid.Scroll != fyne.ScrollHorizontalOnly {
		t.Fatalf("tool-output grid Scroll = %v, want ScrollHorizontalOnly", grid.Scroll)
	}
	// The grid must not demand the full width of its longest line; otherwise the
	// vertical-only outer scroll would propagate that width and widen the window.
	if w := grid.MinSize().Width; w > 600 {
		t.Fatalf("tool-output grid MinSize width = %.0f; want it bounded so the window keeps its width", w)
	}
}

func TestSetTranscriptReusesExistingToolOutputWidget(t *testing.T) {
	p := &popupWindow{}
	p.buildOutput()

	blocks := []transcriptBlock{
		{Kind: transcriptBlockProse, Text: "Running unix..."},
		{Kind: transcriptBlockToolOutput, Chunks: []string{"line1\n"}},
	}
	p.setTranscript(blocks)

	if len(p.transcriptViews) != 2 {
		t.Fatalf("transcript view count = %d, want 2", len(p.transcriptViews))
	}
	grid := p.transcriptViews[1].textGrid
	rich := p.transcriptViews[0].richText
	if grid == nil || rich == nil {
		t.Fatal("expected rich text and text grid transcript views")
	}

	updated := []transcriptBlock{
		{Kind: transcriptBlockProse, Text: "Running unix..."},
		{Kind: transcriptBlockToolOutput, Chunks: []string{"line1\n", "line2\n"}},
	}
	p.setTranscript(updated)

	if p.transcriptViews[1].textGrid != grid {
		t.Fatal("tool-output updates should reuse the existing text grid")
	}
	if p.transcriptViews[0].richText != rich {
		t.Fatal("unchanged prose blocks should reuse the existing rich text widget")
	}
	if got := p.transcriptViews[1].textGrid.Text(); got != "line1\nline2\n" {
		t.Fatalf("text grid content = %q, want updated output", got)
	}
	if p.transcriptViews[1].chunks != 2 {
		t.Fatalf("tracked chunk count = %d, want 2", p.transcriptViews[1].chunks)
	}
}

func TestAppendTextGridTextNoRefreshPreservesChunkBytes(t *testing.T) {
	grid := widget.NewTextGrid()

	appendTextGridTextNoRefresh(grid, "line1\n")
	appendTextGridTextNoRefresh(grid, "line2")

	if got := grid.Text(); got != "line1\nline2" {
		t.Fatalf("text grid text = %q, want exact appended chunks", got)
	}
}

func TestPromptInputRowsExpandsForNewlinesAndWrappedText(t *testing.T) {
	charWidth := fyne.MeasureText("M", theme.TextSize(), fyne.TextStyle{Monospace: true}).Width
	widthForTenColumns := inputTextInset + charWidth*10

	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "single line", value: "short", want: 1},
		{name: "explicit newline", value: "one\ntwo", want: 2},
		{name: "wrapped line", value: "12345678901", want: 2},
		{name: "capped", value: "one\ntwo\nthree\nfour", want: maxInputRows},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := promptInputRows(tt.value, widthForTenColumns); got != tt.want {
				t.Fatalf("promptInputRows(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestResizeInputDoesNotOverrideManualWindowResize(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	g := NewApp(&recordingService{}, voiceGUIConfig{}, AppOptions{FyneApp: a})
	p := g.popup
	manualSize := fyne.NewSize(defaultWindowWidth-140, minWindowHeight-80)
	p.window.Resize(manualSize)

	p.resizeInput("a short prompt")

	if got := p.window.Canvas().Size(); got != manualSize {
		t.Fatalf("window size after resizeInput = %v, want %v", got, manualSize)
	}
}

func TestBrandThemeUsesAccentColorForNativeEntryCursor(t *testing.T) {
	if got := newBrandTheme().Color(theme.ColorNamePrimary, theme.VariantDark); got != brandAccentGreen {
		t.Fatalf("ColorNamePrimary = %v, want %v", got, brandAccentGreen)
	}
	if got := newBrandTheme().Color(theme.ColorNamePrimary, theme.VariantLight); got != brandLightPalette.accentGreen {
		t.Fatalf("light ColorNamePrimary = %v, want %v", got, brandLightPalette.accentGreen)
	}
}

func TestPromptEntryThemeForcesVisibleNativeCursorWidth(t *testing.T) {
	if got := newBrandTheme().Size(theme.SizeNameInputBorder); got != 0 {
		t.Fatalf("brand theme input border = %v, want 0", got)
	}
	if got := (promptEntryTheme{Theme: newBrandTheme()}).Size(theme.SizeNameInputBorder); got != promptNativeCursorWidth {
		t.Fatalf("prompt entry theme input border = %v, want %v", got, promptNativeCursorWidth)
	}
}

func TestUnwrapMarkdownFence(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		changed bool
	}{
		{
			name:    "plain text unchanged",
			input:   "Hello, world!",
			want:    "Hello, world!",
			changed: false,
		},
		{
			name:    "regular code block unchanged",
			input:   "```python\nprint('hi')\n```",
			want:    "```python\nprint('hi')\n```",
			changed: false,
		},
		{
			name:    "unwrap markdown fence",
			input:   "```markdown\n# Title\n\nSome text.\n```",
			want:    "# Title\n\nSome text.",
			changed: true,
		},
		{
			name:    "unwrap md fence",
			input:   "```md\n# Title\n```",
			want:    "# Title",
			changed: true,
		},
		{
			name:    "case insensitive",
			input:   "```Markdown\n# Title\n```",
			want:    "# Title",
			changed: true,
		},
		{
			name:    "preserves inner code blocks",
			input:   "```markdown\n# Title\n\n```bash\necho hi\n```\n\n## End\n```",
			want:    "# Title\n\n```bash\necho hi\n```\n\n## End",
			changed: true,
		},
		{
			name:    "no closing fence unchanged",
			input:   "```markdown\n# Title\nNo closing fence",
			want:    "```markdown\n# Title\nNo closing fence",
			changed: false,
		},
		{
			name:    "handles surrounding whitespace",
			input:   "\n  ```markdown\n# Title\n```  \n",
			want:    "# Title",
			changed: true,
		},
		{
			name:    "empty content between fences unchanged",
			input:   "```markdown\n```",
			want:    "```markdown\n```",
			changed: false,
		},
		{
			name:    "untagged fence unchanged",
			input:   "```\n# Title\n```",
			want:    "```\n# Title\n```",
			changed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unwrapMarkdownFence(tt.input)
			if got != tt.want {
				t.Errorf("unwrapMarkdownFence():\n got  %q\n want %q", got, tt.want)
			}
		})
	}
}
