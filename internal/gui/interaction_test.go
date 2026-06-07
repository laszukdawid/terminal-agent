package gui

import (
	"testing"

	appservice "github.com/laszukdawid/terminal-agent/internal/app"
)

func TestResolveInteractionRunsReplyOnce(t *testing.T) {
	g, _ := newRecordingApp(t)
	dlg := g.popup.newClarificationDialog("q?", func(string) {}, func() {})
	g.beginInteraction(dlg)

	calls := 0
	g.resolveInteraction(func() { calls++ })
	g.resolveInteraction(func() { calls++ })

	if calls != 1 {
		t.Fatalf("reply ran %d times, want 1", calls)
	}
	if g.pending != nil {
		t.Fatal("pending should be cleared after resolve")
	}
}

func TestDismissThenResolveIsNoop(t *testing.T) {
	g, _ := newRecordingApp(t)
	dlg := g.popup.newClarificationDialog("q?", func(string) {}, func() {})
	g.beginInteraction(dlg)

	g.dismissInteraction()
	called := false
	g.resolveInteraction(func() { called = true })

	if called {
		t.Fatal("reply must not run after dismiss")
	}
	if g.pending != nil {
		t.Fatal("pending should be nil after dismiss")
	}
}

func TestRequestClarificationOpensPending(t *testing.T) {
	g, _ := newRecordingApp(t)
	g.requestClarification(&appservice.TaskClarificationEvent{
		Question: "staging or prod?",
		Reply:    func(string) error { return nil },
	})
	if g.pending == nil {
		t.Fatal("expected a pending interaction after requestClarification")
	}
}

func TestFinishRunDismissesPendingInteraction(t *testing.T) {
	g, _ := newRecordingApp(t)
	g.requestClarification(&appservice.TaskClarificationEvent{
		Question: "q?",
		Reply:    func(string) error { return nil },
	})

	g.finishRun(nil)

	if g.pending != nil {
		t.Fatal("finishRun must dismiss any pending interaction")
	}
}

func TestHideDismissesPendingInteraction(t *testing.T) {
	g, _ := newRecordingApp(t)
	g.requestClarification(&appservice.TaskClarificationEvent{
		Question: "q?",
		Reply:    func(string) error { return nil },
	})

	g.Hide()

	if g.pending != nil {
		t.Fatal("Hide must dismiss any pending interaction")
	}
}

func TestQuitDismissesPendingInteraction(t *testing.T) {
	g, _ := newRecordingApp(t)
	g.quit = func() {} // don't tear down the shared test app
	g.requestClarification(&appservice.TaskClarificationEvent{
		Question: "q?",
		Reply:    func(string) error { return nil },
	})

	g.Quit()

	if g.pending != nil {
		t.Fatal("Quit must dismiss any pending interaction")
	}
}

func TestFinishRunFlushesDirtyOutput(t *testing.T) {
	g, _ := newRecordingApp(t)
	g.state.isRunning = true
	g.state.cancelFunc = func() {}
	g.state.output = "partial transcript"
	g.state.outputDirty = true

	g.finishRun(nil)

	if g.state.outputDirty {
		t.Fatal("finishRun must flush dirty output (clear the dirty flag)")
	}
}

func TestReplyAfterRunEndIsInert(t *testing.T) {
	g, _ := newRecordingApp(t)
	replied := false
	clar := &appservice.TaskClarificationEvent{
		Question: "q?",
		Reply:    func(string) error { replied = true; return nil },
	}
	g.requestClarification(clar)
	g.finishRun(nil) // run ends while the dialog is still open

	// A late Send routed through the controller must be inert.
	g.resolveInteraction(func() { _ = clar.Reply("late") })

	if replied {
		t.Fatal("a reply after the run ended must not run")
	}
}
