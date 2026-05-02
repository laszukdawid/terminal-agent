package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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
	quit    func()
	stopThinking chan struct{}
}

func NewApp(service appservice.Service, cfg config.Config) *App {
	fyneApp := app.NewWithID("com.terminal-agent.popup")
	gui := &App{
		fyneApp: fyneApp,
		service: service,
		cfg:     cfg,
		state:   &state{},
		quit:    fyneApp.Quit,
		stopThinking: make(chan struct{}),
	}
	if icon, err := loadAppIcon(); err == nil {
		fyneApp.SetIcon(icon)
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
	if g.state.isVisible {
		g.popup.window.RequestFocus()
		g.FocusInput()
		return
	}

	if g.state.errorText != "" && !g.state.isRunning {
		g.state.errorText = ""
	}
	if !g.state.isRunning {
		g.state.status = ""
	}
	g.state.isVisible = true
	g.popup.resizeInput(g.state.input)
	g.render()
	g.popup.window.Show()
	g.popup.window.RequestFocus()
	g.FocusInput()
}

func (g *App) Hide() {
	if g.state.cancelFunc != nil {
		g.stopThinkingIndicator()
		g.state.cancelFunc()
		g.state.clearRunning()
		if g.state.status != "" {
			g.state.status = ""
		}
	}
	g.state.isVisible = false
	g.popup.window.Hide()
}

func (g *App) startThinking() {
	stop := make(chan struct{})
	g.stopThinking = stop
	go func() {
		ticker := time.NewTicker(450 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fyne.Do(func() {
					if !g.state.isRunning || g.state.statusBase == "" {
						return
					}
					g.state.advanceThinking(g.state.statusBase)
					g.render()
				})
			case <-stop:
				return
			}
		}
	}()
}

func (g *App) stopThinkingIndicator() {
	select {
	case <-g.stopThinking:
	default:
		close(g.stopThinking)
	}
}

func (g *App) FocusInput() {
	g.popup.window.Canvas().Focus(g.popup.input)
}

func (g *App) wire() {
	g.popup.onSubmit = g.submit
	g.popup.onCancel = g.cancel
	g.popup.onCopy = g.copyOutput
	g.popup.onEscape = func() {
		g.Hide()
	}
	g.popup.onInput = func(value string) {
		g.state.input = value
		g.state.showRequest = g.state.question != "" && value != g.state.question
		if g.state.errorText != "" {
			g.state.errorText = ""
			g.render()
			return
		}
		g.render()
	}

	g.popup.window.SetCloseIntercept(func() {
		g.Hide()
	})
	if desk, ok := g.fyneApp.(desktopApp); ok {
		trayMenu := fyne.NewMenu(
			"Terminal Agent",
			fyne.NewMenuItem("Show", func() {
				g.Show()
			}),
			fyne.NewMenuItem("Hide", func() {
				g.Hide()
			}),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Quit", func() {
				g.quit()
			}),
		)
		desk.SetSystemTrayMenu(trayMenu)
	}
	if desk, ok := g.fyneApp.(interface{ SetIcon(fyne.Resource) }); ok {
		_ = desk
	}
}

func (g *App) render() {
	g.popup.input.SetText(g.state.input)
	g.popup.questionLabel.SetText(g.state.question)
	g.popup.outputLabel.SetText(g.state.output)
	g.popup.outputScroll.SetMinSize(fyne.NewSize(0, g.popup.outputHeight()))
	g.popup.modelLabel.Text = g.cfg.GetDefaultProvider() + " / " + g.cfg.GetDefaultModelId()
	g.popup.modelLabel.Refresh()
	g.popup.answerHeading.Show()
	if g.state.showRequest {
		g.popup.requestHeading.Show()
		g.popup.answerPanel.Objects[1].Show()
	} else {
		g.popup.requestHeading.Hide()
		g.popup.answerPanel.Objects[1].Hide()
	}
	if g.state.question == "" && g.state.output == "" && !g.state.isRunning && g.state.errorText == "" {
		g.popup.answerHeading.Hide()
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

type desktopApp interface {
	SetSystemTrayMenu(*fyne.Menu)
}

func loadAppIcon() (fyne.Resource, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	iconPath := filepath.Join(root, "assets", "icon.png")
	data, err := os.ReadFile(iconPath)
	if err != nil {
		return nil, fmt.Errorf("read icon: %w", err)
	}
	return fyne.NewStaticResource("terminal-agent.png", data), nil
}
