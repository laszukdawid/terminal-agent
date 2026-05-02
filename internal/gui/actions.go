package gui

import (
	"context"
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

	g.state.resetOutput()
	g.state.question = message
	g.state.showRequest = false
	ctx, cancel := context.WithCancel(context.Background())
	g.state.setRunning(cancel)
	g.startThinking()
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
		g.state.clearRunning()
		g.state.errorText = err.Error()
		g.render()
		return
	}

	go g.consumeAskEvents(events)
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
