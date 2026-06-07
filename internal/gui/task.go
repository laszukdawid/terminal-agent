package gui

import (
	"context"

	"fyne.io/fyne/v2"

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
		AutoApprove: g.taskAutoApprove(),
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
					final, _ := eventCopy.ToolInput["final"].(bool)
					g.state.taskCurrentToolFinal = final
					g.state.appendTaskTranscriptLine(eventCopy.Text)
					g.state.outputDirty = true
				}

			case appservice.EventToolProgress:
				if eventCopy.Text != "" {
					g.state.status = eventCopy.Text
				}
				g.state.appendTaskTranscriptLine(formatTaskToolProgress(eventCopy))
				g.state.outputDirty = true

			case appservice.EventOutputDelta:
				g.state.taskSawLiveOutput = true
				if eventCopy.ToolName != "" {
					g.state.taskLiveOutputTools[eventCopy.ToolName] = true
				}
				// final/direct-output tools stream unlimited because their output is
				// the run's answer (matching the CLI). Non-final live output is capped
				// to GetTaskLiveOutputLimit lines. Large final output is bounded only
				// by render throttling here; the proper fix is the monospace segmented
				// transcript tracked in issue #55.
				maxLines := 0
				if !g.state.taskCurrentToolFinal {
					maxLines = g.cfg.GetTaskLiveOutputLimit()
				}
				g.state.openTaskToolBlock(eventCopy.ProcessID, maxLines)
				g.state.appendTaskOutput(eventCopy.Text)
				if eventCopy.ToolName != "" && g.state.taskLiveLimiter != nil && g.state.taskLiveLimiter.Truncated() {
					g.state.taskLiveOutputTruncatedTools[eventCopy.ToolName] = true
				}
				g.state.outputDirty = true
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
				g.requestClarification(eventCopy.Clarification)

			case appservice.EventCompleted:
				g.state.closeTaskToolBlock()
				if shouldAppendTaskFinalOutput(eventCopy, g.state) {
					g.state.appendTaskFinalText(taskFinalOutputText(eventCopy))
				}
				g.state.outputDirty = true
				g.finishRun(nil)

			case appservice.EventFailed:
				g.finishRun(eventCopy.Err)
			}
			g.render()
		})
	}
}

// shouldAppendTaskFinalOutput reports whether task completion should append
// final/raw text to the transcript, given what already streamed live. It avoids
// duplicating direct raw output that was already shown for a tool.
func shouldAppendTaskFinalOutput(event appservice.Event, s *state) bool {
	// Suppress direct raw output only when it already streamed in full. If the
	// live stream was truncated, fall through and append the complete result so
	// nothing is lost (mirrors the CLI's TruncatedTool de-dupe).
	truncated := s.taskLiveOutputTruncatedTools[event.RawOutputTool]
	if event.DirectRawOutput && event.RawOutputTool != "" &&
		s.taskLiveOutputTools[event.RawOutputTool] && !truncated {
		return false
	}
	if event.FinalOutput != "" {
		return true
	}
	// Fall back to raw output when none streamed, or when what streamed was
	// truncated and would otherwise be the only (partial) copy shown.
	return event.RawOutput != "" && (!s.taskSawLiveOutput || truncated)
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

