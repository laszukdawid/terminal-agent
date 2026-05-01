package gui

import (
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	appservice "github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/config"
)

type App struct {
	fyneApp fyne.App
	service appservice.Service
	cfg     config.Config
	state   *state
	popup   *popupWindow
}

func NewApp(service appservice.Service, cfg config.Config) *App {
	fyneApp := app.NewWithID("com.terminal-agent.popup")
	gui := &App{
		fyneApp: fyneApp,
		service: service,
		cfg:     cfg,
		state:   &state{},
	}
	gui.popup = newPopupWindow(fyneApp)
	gui.wire()
	gui.render()
	return gui
}

func (g *App) Run() {
	g.Show()
	g.fyneApp.Run()
}

func (g *App) Show() {
	g.popup.window.Show()
	g.popup.window.RequestFocus()
	g.FocusInput()
}

func (g *App) Hide() {
	g.popup.window.Hide()
}

func (g *App) FocusInput() {
	g.popup.window.Canvas().Focus(g.popup.input)
}

func (g *App) wire() {
	g.popup.onSubmit = g.submit
	g.popup.onCancel = g.cancel
	g.popup.onCopy = g.copyOutput
	g.popup.onEscape = g.Hide
	g.popup.onInput = func(value string) {
		g.state.input = value
		if g.state.errorText != "" {
			g.state.errorText = ""
			g.render()
		}
	}

	g.popup.window.SetCloseIntercept(func() {
		g.fyneApp.Quit()
	})
	if desk, ok := g.fyneApp.(interface{ SetIcon(fyne.Resource) }); ok {
		_ = desk
	}
}

func (g *App) render() {
	g.popup.questionLabel.SetText(g.state.question)
	g.popup.outputLabel.SetText(g.state.output)
	g.popup.modelLabel.SetText(g.cfg.GetDefaultProvider() + " / " + g.cfg.GetDefaultModelId())
	if g.state.question != "" {
		g.popup.answerPanel.Objects[0].Show()
	} else {
		g.popup.answerPanel.Objects[0].Hide()
	}

	status := g.state.status
	if g.state.errorText != "" {
		status = g.state.errorText
	}
	g.popup.statusLabel.SetText(status)

	if g.state.isRunning {
		g.popup.submitButton.Disable()
		g.popup.cancelButton.Enable()
	} else {
		g.popup.submitButton.Enable()
		g.popup.cancelButton.Disable()
	}

	if g.state.output != "" {
		g.popup.copyButton.Enable()
	} else {
		g.popup.copyButton.Disable()
	}

	g.popup.outputScroll.ScrollToBottom()
}

func memoryPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".local", "share", "terminal-agent", "memory.jsonl")
}
