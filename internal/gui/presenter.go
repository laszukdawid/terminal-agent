package gui

import (
	"fyne.io/fyne/v2"
	appservice "github.com/laszukdawid/terminal-agent/internal/app"
)

func (g *App) consumeAskEvents(events <-chan appservice.Event) {
	for event := range events {
		eventCopy := event
		fyne.Do(func() {
			switch eventCopy.Type {
			case appservice.EventStarted:
				g.state.status = "Thinking..."
			case appservice.EventOutputDelta:
				g.state.output += eventCopy.Text
				g.state.status = "Responding..."
			case appservice.EventCompleted:
				g.state.output = eventCopy.FinalOutput
				g.state.status = ""
				g.state.clearRunning()
			case appservice.EventFailed:
				g.state.errorText = eventCopy.Err.Error()
				g.state.status = ""
				g.state.clearRunning()
			}
			g.render()
		})
	}
}
