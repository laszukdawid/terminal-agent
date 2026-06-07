package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
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
	if p.errorLabel.Wrapping != fyne.TextWrapBreak {
		t.Fatalf("errorLabel.Wrapping = %v, want %v", p.errorLabel.Wrapping, fyne.TextWrapBreak)
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

func TestPopupEntryFocusCursorStaysInsideScrolledPrompt(t *testing.T) {
	entry := &popupEntry{}
	entry.ExtendBaseWidget(entry)
	entry.MultiLine = true
	entry.focusCursor = canvas.NewRectangle(brandAccentGreen)
	entry.CursorRow = maxInputRows + 1

	textSize := fyne.MeasureText("M", theme.TextSize(), entry.TextStyle)
	entry.Resize(fyne.NewSize(200, textSize.Height*maxInputRows))

	entry.positionFocusCursor()

	maxY := entry.Size().Height - entry.focusCursor.Size().Height
	if got := entry.focusCursor.Position().Y; got < 0 || got > maxY {
		t.Fatalf("focus cursor Y = %v, want within [0, %v]", got, maxY)
	}
}

func TestPopupEntryVisibleTopRowPersistsWhenCursorMovesWithinScrolledPrompt(t *testing.T) {
	entry := &popupEntry{Entry: widget.Entry{Text: "one\ntwo\nthree\nfour"}}
	entry.CursorRow = 3
	entry.updateVisibleTopRow()
	if entry.visibleTopRow != 1 {
		t.Fatalf("visibleTopRow at bottom = %d, want 1", entry.visibleTopRow)
	}

	entry.CursorRow = 2
	entry.updateVisibleTopRow()
	if entry.visibleTopRow != 1 {
		t.Fatalf("visibleTopRow after moving up within window = %d, want 1", entry.visibleTopRow)
	}

	entry.CursorRow = 0
	entry.updateVisibleTopRow()
	if entry.visibleTopRow != 0 {
		t.Fatalf("visibleTopRow after moving above window = %d, want 0", entry.visibleTopRow)
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
