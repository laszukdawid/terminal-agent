package gui

import (
	"context"
	"errors"
	"strings"

	"fyne.io/fyne/v2"
	appservice "github.com/laszukdawid/terminal-agent/internal/app"
)

func (g *App) consumeAskEvents(events <-chan appservice.Event) {
	for event := range events {
		eventCopy := event
		fyne.Do(func() {
			switch eventCopy.Type {
			case appservice.EventStarted:
				g.state.status = "thinking"
			case appservice.EventOutputDelta:
				g.state.output += eventCopy.Text
				g.renderOutput()
				g.popup.outputScroll.ScrollToBottom()
				g.state.status = "responding"
			case appservice.EventCompleted:
				g.state.output = eventCopy.FinalOutput
				g.renderOutput()
				g.stopIndicatorAnimation()
				g.state.clearRunning()
				g.FocusInput()
			case appservice.EventFailed:
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

func isCanceledError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "context canceled") || strings.Contains(message, "context cancelled")
}
