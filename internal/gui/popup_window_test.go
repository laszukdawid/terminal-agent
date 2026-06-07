package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

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
