package gui

import (
	"context"
	"fmt"
	"strings"

	appservice "github.com/laszukdawid/terminal-agent/internal/app"
)

func (g *App) submit() {
	if g.state.isRunning {
		return
	}

	message := strings.TrimSpace(g.popup.input.Text)
	if message == "" {
		g.state.errorText = "Question cannot be empty."
		g.render()
		return
	}

	promptChanged := message != g.state.question
	hadPreviousResponse := strings.TrimSpace(g.state.output) != ""
	g.state.resetOutput()
	g.state.question = message
	g.state.showRequest = false
	ctx, cancel := context.WithCancel(context.Background())
	g.state.setRunning(cancel)
	g.state.responsePrefix = responseMarker(message, promptChanged, hadPreviousResponse)
	g.renderOutput()
	g.startIndicator()
	g.render()

	events, err := g.service.AskEvents(ctx, appservice.AskRequest{
		Message:    message,
		Provider:   g.cfg.GetDefaultProvider(),
		Model:      g.cfg.GetDefaultModelId(),
		UseMemory:  g.cfg.GetMemory(),
		MemoryPath: memoryPath(),
		WorkingDir: g.cfg.GetWorkingDir(),
		Stream:     true,
		Config:     g.cfg,
	})
	if err != nil {
		g.stopIndicatorAnimation()
		g.state.clearRunning()
		g.state.errorText = err.Error()
		g.render()
		return
	}

	go g.consumeAskEvents(events)
}

// responseMarker builds the label rendered above a response. It is only shown
// when the prompt changed from the previous request; re-submitting the same
// prompt yields just the response. A separator is prefixed when the marker
// replaces a previous response, so a changed prompt is visually distinct.
func responseMarker(prompt string, changed, afterPrevious bool) string {
	if !changed {
		return ""
	}
	marker := fmt.Sprintf("Response to: %s\n\n", prompt)
	if afterPrevious {
		marker = "---\n\n" + marker
	}
	return marker
}

func (g *App) cancel() {
	if g.state.cancelFunc != nil {
		g.state.cancelFunc()
	}
}

func (g *App) copyOutput() {
	text := g.state.output
	if text == "" {
		text = g.state.question
	}
	if text != "" {
		g.fyneApp.Clipboard().SetContent(text)
	}
}
