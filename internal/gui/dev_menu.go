package gui

import (
	"context"

	appservice "github.com/laszukdawid/terminal-agent/internal/app"
)

// openTestMenu shows the dev-only test dialog. Each entry exercises a GUI
// rendering path without requiring the user to manually reproduce it.
func (g *App) openTestMenu() {
	g.popup.showTestDialog([]devTest{
		{
			name: "Exhaustive markdown",
			run: func() {
				g.showCannedOutput("Markdown feature test", exhaustiveMarkdown)
			},
		},
		{
			name: "Invalid provider error",
			run:  g.showInvalidProviderError,
		},
	})
}

// showCannedOutput places static content in the response area as if a run had
// completed, without contacting any provider. It is useful for renderer-only
// scenarios such as markdown coverage.
func (g *App) showCannedOutput(question, output string) {
	if g.state.isRunning {
		g.cancel()
	}
	g.state.resetOutput()
	g.state.question = question
	g.state.output = output
	g.state.markCompleted()
	g.renderResponse()
	g.render()
}

// showInvalidProviderError exercises the same error path used by real ask
// requests without changing the user's saved provider settings.
func (g *App) showInvalidProviderError() {
	if g.state.isRunning {
		g.cancel()
	}

	question := "Dev test: invalid provider"
	g.state.resetOutput()
	g.state.question = question

	events, err := g.service.AskEvents(context.Background(), appservice.AskRequest{
		Message:    question,
		Provider:   "terminal-agent-dev-invalid-provider",
		Model:      g.cfg.GetDefaultModelId(),
		UseMemory:  g.cfg.GetMemory(),
		MemoryPath: memoryPath(),
		WorkingDir: g.cfg.GetWorkingDir(),
		Stream:     true,
		Config:     g.cfg,
	})
	if err != nil {
		g.state.errorText = runtimeErrorMessage(err)
		g.renderResponse()
		g.render()
		return
	}

	go g.consumeAskEvents(events)
	g.render()
}
