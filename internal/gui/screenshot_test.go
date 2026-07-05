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

	appservice "github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/routines"
	"github.com/laszukdawid/terminal-agent/internal/taskmemory"
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

	// Redirect task memory the same way and seed representative records so the
	// Task Memory views render deterministic, illustrative content.
	seedScreenshotTaskMemories(t)

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
	// The form is a local dialog with no handle on g; pop it off the overlay
	// stack so it does not linger over the captures that follow.
	for win.Canvas().Overlays().Top() != nil {
		win.Canvas().Overlays().Remove(win.Canvas().Overlays().Top())
	}

	// Routine log viewer: a run summary rendered as markdown, with the run
	// metadata collapsed into a two-column "Details" block and the output set
	// apart in a highlighted box.
	if refs, err := g.routineService.Logs(context.Background(), "daily-standup"); err == nil {
		for _, ref := range refs {
			if ref.IsResult {
				g.openRoutineLog(ref)
				capture("gui-routine-log.png")
				g.popup.dismissRoutineDetail()
				break
			}
		}
	}

	// Task memory list: recent indexed records with the search row and stats line.
	g.setMode(guiModeMemory)
	capture("gui-memory.png")

	// Task memory detail: a whole run with its request, steps, and answer.
	if records, _, err := fetchTaskMemories(""); err == nil && len(records) > 0 {
		if groups := groupMemoryRecords(records); len(groups) > 0 {
			g.popup.showMemoryRunDetail(groups[0])
			capture("gui-memory-detail.png")
			g.popup.dismissMemoryDetail()
		}
	}
}

// seedScreenshotTaskMemories redirects the task memory database to a temp path
// and indexes representative records for the Task Memory screenshots.
func seedScreenshotTaskMemories(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "task_memory.db")
	t.Setenv(appservice.TaskMemoryPathEnv, path)
	store, err := taskmemory.Open(path)
	if err != nil {
		t.Fatalf("seed task memories: %v", err)
	}
	defer store.Close()

	base := time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC)
	records := []taskmemory.Record{
		{
			Kind:           taskmemory.KindToolSuccess,
			OS:             "darwin",
			Arch:           "arm64",
			Shell:          "zsh",
			CWD:            "/Users/demo/projects/terminal-agent",
			ToolName:       "unix",
			ToolInput:      map[string]any{"command": "git log --oneline -5"},
			FinalOutput:    "447a7cf feat: skills support\n25bbb3f chore: disable snapcraft",
			SearchableText: "Kind: tool_success\nUser request: show my recent commits\nTool: unix\nInput: {\"command\":\"git log --oneline -5\"}\nOutput: 447a7cf feat: skills support",
			SourceRunID:    "task-a1b2c3",
			SourceSeq:      2,
			CreatedAt:      base,
		},
		{
			Kind:           taskmemory.KindTaskCompletion,
			OS:             "darwin",
			Arch:           "arm64",
			Shell:          "zsh",
			CWD:            "/Users/demo/projects/terminal-agent",
			FinalOutput:    "You have 5 recent commits; the newest adds skills support.",
			SearchableText: "Kind: task_completion\nUser request: show my recent commits\nFinal answer: You have 5 recent commits; the newest adds skills support.",
			SourceRunID:    "task-a1b2c3",
			CreatedAt:      base.Add(time.Minute),
		},
		{
			Kind:           taskmemory.KindToolFailure,
			OS:             "darwin",
			Arch:           "arm64",
			Shell:          "zsh",
			CWD:            "/Users/demo",
			ToolName:       "unix",
			ToolInput:      map[string]any{"command": "docker ps"},
			SearchableText: "Kind: tool_failure\nUser request: list running containers\nTool: unix\nInput: {\"command\":\"docker ps\"}\nError: Cannot connect to the Docker daemon",
			SourceRunID:    "task-d4e5f6",
			SourceSeq:      1,
			CreatedAt:      base.Add(-2 * time.Hour),
		},
	}
	for _, rec := range records {
		if err := store.Add(context.Background(), rec); err != nil {
			t.Fatalf("seed task memories: %v", err)
		}
	}
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

	// A result summary so the detail view's run-log list has an entry and the log
	// viewer has representative content (full run metadata plus a short output).
	logDir := routines.LogDir("daily-standup")
	must(os.MkdirAll(logDir, 0o755))
	must(os.WriteFile(filepath.Join(logDir, "2026-06-07T09-00-00_success_a1b2c3.md"),
		[]byte("# Routine: Daily standup\n\n"+
			"- ID: daily-standup\n- Schedule: weekdays at 09:00\n- Trigger: scheduled\n- Provider/Model: openai / gpt-4o-mini\n"+
			"- Started: 2026-06-07T09:00:00Z\n- Ended: 2026-06-07T09:00:42Z\n- Duration: 42s\n- Status: success\n- Tokens (est.): 1840 / 1000000\n\n"+
			"## Output\n\nHighlights from yesterday's commits:\n\n- Shipped the routines scheduler daemon\n- Fixed the detail overlay backdrop on resize\n- Densified the routine list cards\n"), 0o644))
}
