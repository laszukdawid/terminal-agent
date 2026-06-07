package gui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	appservice "github.com/laszukdawid/terminal-agent/internal/app"
)

// pendingInteraction is the single in-flight, run-owned user interaction (a Task
// clarification today; a permission confirmation in a future release). The agent
// blocks until it is answered, so at most one is outstanding at a time.
//
// Invariant: every access happens on the Fyne UI thread (event handling runs
// inside fyne.Do; dialog button callbacks fire on the UI thread), so the `done`
// flag is a sufficient guard without locking. Do not call these off-thread.
type pendingInteraction struct {
	dialog dialog.Dialog
	done   bool
}

// beginInteraction shows dlg as the run's single pending interaction, dismissing
// any previous one defensively.
func (g *App) beginInteraction(dlg dialog.Dialog) {
	g.dismissInteraction()
	g.pending = &pendingInteraction{dialog: dlg}
	dlg.Show()
}

// resolveInteraction runs reply exactly once for the active interaction, then
// tears it down. A call after the interaction was already resolved or dismissed
// is a no-op, so a late button press can never mutate finalized run state.
func (g *App) resolveInteraction(reply func()) {
	p := g.pending
	if p == nil || p.done {
		return
	}
	p.done = true
	p.dialog.Hide()
	g.pending = nil
	if reply != nil {
		reply()
	}
}

// dismissInteraction tears down the active interaction without replying. Called
// when the run ends for any reason; the agent observes context cancellation and
// applies its own safe default (the action is not executed).
func (g *App) dismissInteraction() {
	p := g.pending
	if p == nil || p.done {
		return
	}
	p.done = true
	p.dialog.Hide()
	g.pending = nil
}

// requestClarification opens the ask_user clarification dialog as the run's
// pending interaction. Send replies with the answer; Cancel cancels the run.
func (g *App) requestClarification(clarification *appservice.TaskClarificationEvent) {
	if clarification == nil {
		return
	}
	dlg := g.popup.newClarificationDialog(
		clarification.Question,
		func(answer string) {
			g.resolveInteraction(func() {
				if err := clarification.Reply(answer); err != nil {
					g.state.errorText = runtimeErrorMessage(err)
					g.render()
				}
			})
		},
		func() {
			g.resolveInteraction(func() { g.cancel() })
		},
	)
	g.beginInteraction(dlg)
}

// newClarificationDialog builds (without showing) the clarification dialog. The
// buttons invoke the supplied callbacks; lifecycle (show/hide) is owned by the
// interaction controller, so the buttons must not hide the dialog themselves.
func (p *popupWindow) newClarificationDialog(question string, onSend func(string), onCancel func()) dialog.Dialog {
	q := widget.NewLabel(question)
	q.Wrapping = fyne.TextWrapWord

	answer := widget.NewMultiLineEntry()
	answer.SetPlaceHolder("Type your answer")
	answer.Wrapping = fyne.TextWrapWord

	send := widget.NewButton("Send", func() { onSend(strings.TrimSpace(answer.Text)) })
	send.Importance = widget.HighImportance
	cancel := widget.NewButton("Cancel", func() { onCancel() })

	footer := container.NewHBox(layout.NewSpacer(), cancel, send)
	content := container.NewVBox(q, answer, footer)

	dlg := dialog.NewCustomWithoutButtons("Clarification", content, p.window)
	dlg.Resize(fyne.NewSize(480, 0))
	return dlg
}
