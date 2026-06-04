package commands

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPatternEditorBuildAction(t *testing.T) {
	t.Run("wraps command in tool expression", func(t *testing.T) {
		editor := NewPatternEditor("unix", "find . -type f", &bytes.Buffer{})
		assert.Equal(t, `unix("find . -type f")`, editor.buildAction("find . -type f"))
	})

	t.Run("wildcarded pattern", func(t *testing.T) {
		editor := NewPatternEditor("unix", "find . -type f", &bytes.Buffer{})
		assert.Equal(t, `unix("find *")`, editor.buildAction("find *"))
	})

	t.Run("escapes quotes in command", func(t *testing.T) {
		editor := NewPatternEditor("unix", `echo "hello"`, &bytes.Buffer{})
		assert.Equal(t, `unix("echo \"hello\"")`, editor.buildAction(`echo "hello"`))
	})
}

func TestPatternEditorLevels(t *testing.T) {
	t.Run("simple command levels", func(t *testing.T) {
		editor := NewPatternEditor("unix", "find . -maxdepth 2 -type d", &bytes.Buffer{})
		assert.Equal(t, []string{
			"find . -maxdepth 2 -type d",
			"find . -maxdepth 2 *",
			"find . *",
			"find *",
		}, editor.levels)
	})

	t.Run("piped command levels", func(t *testing.T) {
		editor := NewPatternEditor("unix", "find . -type f | sort | head -20", &bytes.Buffer{})
		assert.Equal(t, []string{
			"find . -type f | sort | head -20",
			"find . -type f | sort | head *",
			"find . -type f | sort *",
			"find . -type f *",
			"find . *",
			"find *",
		}, editor.levels)
	})

	t.Run("single command has one level", func(t *testing.T) {
		editor := NewPatternEditor("unix", "ls", &bytes.Buffer{})
		assert.Equal(t, []string{"ls"}, editor.levels)
	})

	t.Run("starts at most specific", func(t *testing.T) {
		editor := NewPatternEditor("unix", "find . -type d", &bytes.Buffer{})
		assert.Equal(t, 0, editor.pos)
		assert.Equal(t, "find . -type d", editor.currentPattern())
	})
}

func TestPatternEditorNavigation(t *testing.T) {
	t.Run("pos moves through levels", func(t *testing.T) {
		editor := NewPatternEditor("unix", "find . -maxdepth 2 -type d", &bytes.Buffer{})

		assert.Equal(t, "find . -maxdepth 2 -type d", editor.currentPattern())

		editor.pos = 1
		assert.Equal(t, "find . -maxdepth 2 *", editor.currentPattern())

		editor.pos = 2
		assert.Equal(t, "find . *", editor.currentPattern())

		editor.pos = 3
		assert.Equal(t, "find *", editor.currentPattern())
	})

	t.Run("out of bounds returns exact command", func(t *testing.T) {
		editor := NewPatternEditor("unix", "find . -type d", &bytes.Buffer{})
		editor.pos = -1
		assert.Equal(t, "find . -type d", editor.currentPattern())
		editor.pos = 100
		assert.Equal(t, "find . -type d", editor.currentPattern())
	})
}

func TestPatternEditorSingleLevel(t *testing.T) {
	t.Run("run returns exact for single-token command", func(t *testing.T) {
		editor := NewPatternEditor("unix", "ls", &bytes.Buffer{})
		result, err := editor.Run()
		assert.NoError(t, err)
		assert.Equal(t, `unix("ls")`, result)
	})
}

func TestInteractiveConfirmationRenderMultilineCommand(t *testing.T) {
	var output bytes.Buffer
	ic := newInteractiveConfirmation("", os.Stdin, &output)
	ic.command = "first line\n  second line\nthird line"
	ic.termWidth = 200

	ic.render()

	text := output.String()
	assert.Contains(t, text, "  first line\r\n  ")
	assert.Contains(t, text, "  second line\r\n  third line")
	assert.NotContains(t, text, "first line\n  second")
	assert.Equal(t, 7, ic.lastVisualLines)
}
