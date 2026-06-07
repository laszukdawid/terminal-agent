package gui

import (
	"context"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	appservice "github.com/laszukdawid/terminal-agent/internal/app"
)

const exportFileName = "terminal-agent-response.md"

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
	ctx, cancel := context.WithCancel(context.Background())
	g.state.setRunning(cancel)
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
		g.state.errorText = runtimeErrorMessage(err)
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
	text := responseCopyText(g.state)
	if text != "" {
		g.fyneApp.Clipboard().SetContent(text)
	}
}

// exportOutput writes the current response (with a small provenance header) to
// a file chosen by the user. It mirrors the "EXPORT LOG" affordance from the
// brand: a lightweight terminal-output action rather than a document toolbar.
func (g *App) exportOutput() {
	text := responseCopyText(g.state)
	if text == "" {
		return
	}
	content := g.exportContent(text)
	save := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil || w == nil {
			return
		}
		defer w.Close()
		if _, werr := w.Write([]byte(content)); werr != nil {
			g.state.errorText = "Export failed: " + werr.Error()
			g.render()
		}
	}, g.popup.window)
	save.SetFileName(exportFileName)
	save.Show()
}

func (g *App) exportContent(body string) string {
	header := fmt.Sprintf(
		"# Terminal Agent response\n\nprovider/model: %s / %s\ngenerated: %s\n\n---\n\n",
		g.cfg.GetDefaultProvider(),
		g.cfg.GetDefaultModelId(),
		g.state.completedAt.Format("2006-01-02 15:04:05"),
	)
	question := strings.TrimSpace(g.state.question)
	if question == "" {
		return header + body + "\n"
	}
	return header + "# Ask\n\n" + question + "\n\n---\n\n# Response\n\n" + body + "\n"
}
