package gui

import (
	"context"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/laszukdawid/terminal-agent/internal/agent"
	appservice "github.com/laszukdawid/terminal-agent/internal/app"
)

// submitTask dispatches a Task run. GUI Task uses AutoApprove so the run is not
// blocked on confirmation prompts; it deliberately passes none of the Ask-only
// memory/context/prompt features. Permission rule sets still load from
// global/local config in the task layer.
func (g *App) submitTask(ctx context.Context, message string) {
	events, err := g.service.TaskEvents(ctx, appservice.TaskRequest{
		Message:     message,
		Provider:    g.cfg.GetDefaultProvider(),
		Model:       g.cfg.GetDefaultModelId(),
		WorkingDir:  g.cfg.GetWorkingDir(),
		AutoApprove: true,
		Device:      g.cfg.GetDevice(),
		Timeout:     g.cfg.GetTaskTimeout(),
		Config:      g.cfg,
	})
	if err != nil {
		g.failRunSetup(err)
		return
	}

	go g.consumeTaskEvents(events)
}

// consumeTaskEvents drains the Task event stream, building a rich transcript in
// state.output (status/tool-call lines, progress lines, live tool output, and
// the final answer) while keeping the existing brain/spinner indicators working
// through the shared status surface.
func (g *App) consumeTaskEvents(events <-chan appservice.Event) {
	for event := range events {
		eventCopy := event
		fyne.Do(func() {
			switch eventCopy.Type {
			case appservice.EventStarted:
				g.state.status = "thinking"

			case appservice.EventTaskStatus:
				g.state.status = taskStatusDisplay(eventCopy)
				if isMeaningfulTaskStatus(eventCopy.Status) {
					g.state.appendTaskTranscriptLine(eventCopy.Text)
					g.renderOutput()
					g.popup.outputScroll.ScrollToBottom()
				}

			case appservice.EventToolProgress:
				line := formatTaskToolProgress(eventCopy)
				if eventCopy.Text != "" {
					g.state.status = eventCopy.Text
				}
				g.state.appendTaskTranscriptLine(line)
				g.renderOutput()
				g.popup.outputScroll.ScrollToBottom()

			case appservice.EventOutputDelta:
				g.state.taskSawLiveOutput = true
				if eventCopy.ToolName != "" {
					g.state.taskLiveOutputTools[eventCopy.ToolName] = true
				}
				g.state.openTaskToolBlock(eventCopy.ProcessID)
				g.state.output += eventCopy.Text
				g.renderOutput()
				g.popup.outputScroll.ScrollToBottom()
				g.state.status = "responding"

			case appservice.EventWarning:
				// Warnings are operational/display noise, not task content: surface
				// them transiently on the status line only. They never enter the
				// transcript, never export, and never fail the run.
				if eventCopy.Text != "" {
					g.state.status = "Warning: " + eventCopy.Text
				}

			case appservice.EventConfirmationNeeded:
				// Unreachable while AutoApprove is true; kept as a deadlock backstop
				// so the agent is never left waiting on a reply that cannot arrive.
				if eventCopy.Confirmation != nil {
					_ = eventCopy.Confirmation.Reply(appservice.TaskConfirmationResponse{Allowed: false})
				}

			case appservice.EventClarificationNeeded:
				g.showTaskClarification(eventCopy.Clarification)

			case appservice.EventCompleted:
				g.state.closeTaskToolBlock()
				if shouldAppendTaskFinalOutput(eventCopy, g.state) {
					g.state.appendTaskFinalText(taskFinalOutputText(eventCopy))
				}
				g.state.markCompleted()
				g.renderOutput()
				g.stopIndicatorAnimation()
				g.state.clearRunning()
				g.FocusInput()

			case appservice.EventFailed:
				g.state.markCompleted()
				g.stopIndicatorAnimation()
				if isCanceledError(eventCopy.Err) {
					g.state.errorText = ""
					g.state.status = "Canceled."
				} else {
					g.state.errorText = runtimeErrorMessage(eventCopy.Err)
				}
				g.state.clearRunning()
				g.FocusInput()
			}
			g.render()
		})
	}
}

// shouldAppendTaskFinalOutput reports whether task completion should append
// final/raw text to the transcript, given what already streamed live. It avoids
// duplicating direct raw output that was already shown for a tool.
func shouldAppendTaskFinalOutput(event appservice.Event, s *state) bool {
	if event.DirectRawOutput && event.RawOutputTool != "" && s.taskLiveOutputTools[event.RawOutputTool] {
		return false
	}
	return event.FinalOutput != "" || (!s.taskSawLiveOutput && event.RawOutput != "")
}

// taskFinalOutputText returns the completion text to append: the final answer
// when present, otherwise the raw output as a fallback. Callers should gate on
// shouldAppendTaskFinalOutput first.
func taskFinalOutputText(event appservice.Event) string {
	if event.FinalOutput != "" {
		return event.FinalOutput
	}
	return event.RawOutput
}

// isMeaningfulTaskStatus reports whether a task status phase warrants its own
// transcript line. Tool-call starts are the meaningful phases; other phases
// (thinking, finalizing) only update the transient status surface.
func isMeaningfulTaskStatus(phase string) bool {
	return phase == string(agent.TaskStatusRunningTool)
}

// taskStatusDisplay maps a task status event to the status-surface value. The
// thinking phase keeps the literal "thinking" so the existing brain indicator
// fires; other phases show their human-readable message text.
func taskStatusDisplay(event appservice.Event) string {
	if event.Status == string(agent.TaskStatusThinking) {
		return "thinking"
	}
	if event.Text != "" {
		return event.Text
	}
	return event.Status
}

// formatTaskToolProgress renders a progress line, prefixing the tool name when
// present so the transcript reads "tool: message".
func formatTaskToolProgress(event appservice.Event) string {
	if event.ToolName == "" {
		return event.Text
	}
	if event.Text == "" {
		return event.ToolName
	}
	return event.ToolName + ": " + event.Text
}

// showTaskClarification presents the ask_user clarification dialog. Send replies
// with the entered answer; Cancel cancels the run rather than replying with an
// ambiguous empty answer.
func (g *App) showTaskClarification(clarification *appservice.TaskClarificationEvent) {
	if clarification == nil {
		return
	}

	question := widget.NewLabel(clarification.Question)
	question.Wrapping = fyne.TextWrapWord

	answer := widget.NewMultiLineEntry()
	answer.SetPlaceHolder("Type your answer")
	answer.Wrapping = fyne.TextWrapWord

	var dlg dialog.Dialog
	sendButton := widget.NewButton("Send", func() {
		dlg.Hide()
		if err := clarification.Reply(answer.Text); err != nil {
			g.state.errorText = runtimeErrorMessage(err)
			g.render()
		}
	})
	sendButton.Importance = widget.HighImportance
	cancelButton := widget.NewButton("Cancel", func() {
		dlg.Hide()
		g.cancel()
	})

	footer := container.NewHBox(layout.NewSpacer(), cancelButton, sendButton)
	content := container.NewVBox(question, answer, footer)

	dlg = dialog.NewCustomWithoutButtons("Clarification", content, g.popup.window)
	dlg.Resize(fyne.NewSize(480, 0))
	dlg.Show()
}
