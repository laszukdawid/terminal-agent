package gui

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"github.com/laszukdawid/terminal-agent/internal/routines"
)

type screenshotApp struct {
	fyne.App
	settings *screenshotSettings
}

func (a *screenshotApp) Settings() fyne.Settings {
	return a.settings
}

type screenshotSettings struct {
	fyne.Settings
	variant fyne.ThemeVariant
}

func (s *screenshotSettings) ThemeVariant() fyne.ThemeVariant {
	return s.variant
}

func screenshotThemeVariant() fyne.ThemeVariant {
	if os.Getenv("GUI_THEME_VARIANT") == "light" {
		return theme.VariantLight
	}
	return theme.VariantDark
}

// TestCaptureScreenshots renders the real App in several representative states
// and writes them to PNGs using Fyne's software renderer, so the GUI can be
// inspected and documented without a display server. It is gated behind
// GUI_SCREENSHOTS so it never runs as part of the normal suite.
//
// Usage: GUI_SCREENSHOTS=<abs-dir> go test ./internal/gui/ -run TestCaptureScreenshots
// (the `task screenshots:gui` wrapper sets GUI_SCREENSHOTS to an absolute path).
//
// Note: the pinned Fyne fork's *software* rasterizer historically underflowed on
// some rich-text code-block paths, so the captured states still avoid large live
// tool-output transcripts even though runtime Task output no longer renders via
// fenced markdown.
func TestCaptureScreenshots(t *testing.T) {
	outDir := os.Getenv("GUI_SCREENSHOTS")
	if outDir == "" {
		t.Skip("set GUI_SCREENSHOTS=<dir> to capture GUI screenshots")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir screenshots: %v", err)
	}

	baseApp := test.NewApp()
	wrappedApp := &screenshotApp{
		App: baseApp,
		settings: &screenshotSettings{
			Settings: baseApp.Settings(),
			variant:  screenshotThemeVariant(),
		},
	}

	// Redirect routine storage to a temp dir (away from the user's real routines)
	// and seed representative entries so the Routine views render deterministic,
	// illustrative content. This must run before NewApp, which binds the routine
	// service to the (now redirected) store path.
	routineDir := t.TempDir()
	t.Setenv(routines.DefinitionsFileEnv, filepath.Join(routineDir, "routines.json"))
	t.Setenv(routines.DataDirEnv, filepath.Join(routineDir, "data"))
	seedScreenshotRoutines(t)

	g := NewApp(&recordingService{}, voiceGUIConfig{}, AppOptions{
		AppID:   "terminal-agent-screenshot",
		FyneApp: wrappedApp,
	})
	t.Cleanup(g.Quit) // g.Quit (not fyneApp.Quit) also stops the routine refresh poller

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

	// Ask mode with a rendered markdown response.
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

	// Task mode transcript: a tool-call status line and a final answer.
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
	if g.popup.settings != nil {
		g.popup.settings.dialog.Hide()
	}

	// Routine list: scheduled routines with varied status (active/inactive/error).
	g.setMode(guiModeRoutine)
	capture("gui-routine.png")

	// Routine detail: full prompt, resolved settings, run logs, and actions.
	if view, err := g.routineService.Get(context.Background(), "daily-standup"); err == nil {
		g.showRoutineDetail(view)
		capture("gui-routine-detail.png")
		g.popup.dismissRoutineDetail()
	}

	// Create-routine form (also the edit form), showing what is configurable.
	g.showRoutineForm(nil)
	capture("gui-routine-form.png")
}

// seedScreenshotRoutines writes representative routines and run state into the
// redirected routine stores so the Routine screenshots have illustrative content.
func seedScreenshotRoutines(t *testing.T) {
	t.Helper()
	store := routines.NewStore(routines.DefinitionsPath())
	state := routines.NewStateStore(routines.StatePath())
	must := func(err error) {
		if err != nil {
			t.Fatalf("seed routines: %v", err)
		}
	}
	const model = "gpt-4o-mini"
	lastRun := time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC)
	nextDaily := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)

	must(store.Upsert(routines.Routine{ID: "daily-standup", Name: "Daily standup", Prompt: "Summarize my git commits from yesterday and list the highlights.", Schedule: "0 9 * * 1-5", Model: model, Enabled: true}))
	must(store.Upsert(routines.Routine{ID: "nightly-deps", Name: "Nightly dependency audit", Prompt: "Check for outdated Go modules and known security advisories; summarize findings.", Schedule: "0 2 * * *", Model: model, Enabled: true}))
	must(store.Upsert(routines.Routine{ID: "weekly-digest", Name: "Weekly digest", Prompt: "Draft a weekly project digest from recent repository activity.", Schedule: "0 17 * * 5", Enabled: false}))

	must(state.Record("daily-standup", routines.RunRecord{LastRunAt: lastRun, LastStatus: routines.OutcomeSuccess, LastDuration: "42s", NextRunAt: nextDaily}))
	must(state.Record("nightly-deps", routines.RunRecord{LastRunAt: lastRun.Add(-7 * time.Hour), LastStatus: routines.OutcomeFailed, LastError: "provider error: rate limited", NextRunAt: lastRun.Add(17 * time.Hour)}))

	// A result summary so the detail view's run-log list has an entry.
	logDir := routines.LogDir("daily-standup")
	must(os.MkdirAll(logDir, 0o755))
	must(os.WriteFile(filepath.Join(logDir, "2026-06-07T09-00-00_success_a1b2c3.md"),
		[]byte("# Routine: Daily standup\n\n- Status: success\n- Duration: 42s\n\n## Output\n\nPosted 4 commit highlights to the channel.\n"), 0o644))
}
