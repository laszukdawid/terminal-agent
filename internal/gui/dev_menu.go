package gui

import (
	"context"
	"time"

	"fyne.io/fyne/v2"
	appservice "github.com/laszukdawid/terminal-agent/internal/app"
)

// animationDemoDuration is how long the dev-only "waiting" animation demo runs
// before settling, unless cancelled first with STOP.
const animationDemoDuration = 90 * time.Second

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
		{
			name: "Waiting animation (no LLM)",
			run:  g.runAnimationDemo,
		},
	})
}

// runAnimationDemo drives the in-flight UI state (status indicator, walking
// mascot and data-transfer dots) without contacting any provider, so the
// waiting animation can be observed at length. STOP cancels it early.
func (g *App) runAnimationDemo() {
	if g.state.isRunning {
		g.cancel()
	}

	g.state.resetOutput()
	g.state.question = "Dev: waiting animation"
	ctx, cancel := context.WithCancel(context.Background())
	g.state.setRunning(cancel)
	g.state.status = "responding"
	g.startIndicator()
	g.render()

	go func() {
		timer := time.NewTimer(animationDemoDuration)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
		canceled := ctx.Err() != nil
		fyne.Do(func() {
			g.stopIndicatorAnimation()
			g.state.clearRunning()
			if canceled {
				g.state.status = "Canceled."
			} else {
				g.state.output = "Animation demo finished."
			}
			g.renderResponse()
			g.render()
			g.FocusInput()
		})
	}()
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
