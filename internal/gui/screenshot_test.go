package gui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestCaptureScreenshots renders the real App in several representative states
// and writes them to PNGs using Fyne's software renderer, so the GUI can be
// inspected and documented without a display server. It is gated behind
// GUI_SCREENSHOTS so it never runs as part of the normal suite.
//
// Usage: GUI_SCREENSHOTS=<abs-dir> go test ./internal/gui/ -run TestCaptureScreenshots
// (the `task screenshots:gui` wrapper sets GUI_SCREENSHOTS to an absolute path).
//
// Note: the pinned Fyne fork's *software* rasterizer underflows when drawing the
// brand theme's fenced code blocks (zero-stroke rounded rects), so the captured
// states deliberately avoid triple-backtick code fences. The fenced live-output
// styling renders correctly under the real GL renderer.
func TestCaptureScreenshots(t *testing.T) {
	outDir := os.Getenv("GUI_SCREENSHOTS")
	if outDir == "" {
		t.Skip("set GUI_SCREENSHOTS=<dir> to capture GUI screenshots")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir screenshots: %v", err)
	}

	g := NewApp(&recordingService{}, voiceGUIConfig{}, AppOptions{
		AppID:   "terminal-agent-screenshot",
		FyneApp: test.NewApp(),
	})
	t.Cleanup(func() { g.fyneApp.Quit() })

	win := g.popup.window
	win.Resize(fyne.NewSize(900, 640))

	capture := func(name string) {
		// Showing a previously-hidden section (e.g. the response panel) or a
		// dialog overlay does not relayout the parent in the headless test canvas
		// on its own; a resize nudge forces a full relayout before the paint.
		win.Resize(fyne.NewSize(901, 640))
		win.Resize(fyne.NewSize(900, 640))
		win.Canvas().Capture() // force a fresh layout/paint after the state change
		img := win.Canvas().Capture()
		path := filepath.Join(outDir, name)
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("create %s: %v", path, err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatalf("encode %s: %v", path, err)
		}
		t.Logf("wrote %s", path)
	}

	completedAt := time.Date(2026, 6, 7, 14, 30, 0, 0, time.UTC)

	// Ask mode with a rendered markdown response (no code fence).
	g.setMode(guiModeAsk)
	g.popup.input.SetText("what is terminal agent?")
	g.state.input = "what is terminal agent?"
	g.state.question = "what is terminal agent?"
	g.state.output = "Terminal Agent is a **CLI-first** AI assistant that wraps multiple " +
		"provider APIs and runs agentic tasks directly from your shell.\n\n" +
		"It renders markdown responses and can execute terminal workflows.\n"
	g.state.completedAt = completedAt
	g.state.elapsed = 1200 * time.Millisecond
	g.render()
	capture("gui-ask.png")

	// Task mode transcript: a tool-call status line and a final answer. (Live
	// tool output is wrapped in a fenced code block at runtime; that styling is
	// GL-only, so it is omitted from this software-rendered capture.)
	g.setMode(guiModeTask)
	g.popup.input.SetText("how many Go files are in internal/gui?")
	g.state.input = "how many Go files are in internal/gui?"
	g.state.question = "how many Go files are in internal/gui?"
	g.state.resetOutput()
	g.state.appendTaskTranscriptLine("Running unix: ls internal/gui/*.go | wc -l")
	g.state.appendTaskFinalText("There are 9 Go source files in internal/gui.")
	g.state.completedAt = completedAt
	g.state.elapsed = 2400 * time.Millisecond
	g.render()
	capture("gui-task.png")

	// Settings dialog, shown as a modal overlay over the main window.
	g.openSettings()
	capture("gui-settings.png")
}
