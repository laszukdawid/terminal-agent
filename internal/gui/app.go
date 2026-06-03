package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	appservice "github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/config"
)

type App struct {
	fyneApp       fyne.App
	service       appservice.Service
	cfg           config.Config
	state         *state
	popup         *popupWindow
	quit          func()
	stopIndicator chan struct{}
	version       string
	envResult     EnvironmentLoadResult
}

type AppOptions struct {
	AppID   string
	DevMode bool
	Version string
}

func NewApp(service appservice.Service, cfg config.Config, options AppOptions) *App {
	fyneApp := app.NewWithID(options.AppID)
	gui := &App{
		fyneApp:       fyneApp,
		service:       service,
		cfg:           cfg,
		state:         &state{},
		quit:          fyneApp.Quit,
		stopIndicator: make(chan struct{}),
		version:       options.Version,
	}
	if icon, err := loadAppIcon(); err == nil {
		fyneApp.SetIcon(icon)
	}
	gui.popup = newPopupWindow(fyneApp, options.DevMode)
	gui.wire()
	gui.render()
	return gui
}

func (g *App) Run() {
	g.Show()
	g.fyneApp.Run()
}

func (g *App) LoadInitialEnvironment() {
	g.envResult = LoadEnvironment(g.cfg)
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
		g.state.cancelFunc()
		g.state.clearRunning()
		if g.state.status != "" {
			g.state.status = ""
		}
	}
	g.state.isVisible = false
	g.popup.window.Hide()
}

func (g *App) startIndicator() {
	stop := make(chan struct{})
	g.stopIndicator = stop
	go func() {
		ticker := time.NewTicker(140 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fyne.Do(func() {
					if !g.state.isRunning || g.state.status != "responding" {
						return
					}
					g.state.spinnerFrame = (g.state.spinnerFrame + 1) % len(spinnerFrames)
					g.popup.setStatus(g.state.status, g.state.isRunning, g.state.spinnerFrame)
				})
			case <-stop:
				return
			}
		}
	}()
}

func (g *App) stopIndicatorAnimation() {
	select {
	case <-g.stopIndicator:
	default:
		close(g.stopIndicator)
	}
}

func (g *App) FocusInput() {
	g.popup.window.Canvas().Focus(g.popup.input)
}

func (g *App) wire() {
	g.popup.onSubmit = g.submit
	g.popup.onAction = func() {
		if g.state.isRunning {
			g.cancel()
			return
		}
		g.submit()
	}
	g.popup.onCopy = g.copyOutput
	g.popup.onSettings = g.openSettings
	g.popup.onTest = g.openTestMenu
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
	g.popup.questionLabel.Text = g.state.question
	g.popup.questionLabel.Refresh()
	if !g.state.isRunning {
		g.renderOutput()
	}
	g.popup.modelLabel.SetText(g.cfg.GetDefaultProvider() + " / " + g.cfg.GetDefaultModelId())
	showAnswer := g.state.output != "" || g.state.isRunning || g.state.errorText != ""
	showMeta := g.state.showRequest || showAnswer
	if showMeta {
		g.popup.answerPanel.Show()
	} else {
		g.popup.answerPanel.Hide()
	}
	if showMeta {
		g.popup.answerMeta.Show()
	} else {
		g.popup.answerMeta.Hide()
	}
	if showAnswer {
		g.popup.answerHeader.Show()
	} else {
		g.popup.answerHeader.Hide()
	}
	if g.state.showRequest {
		g.popup.requestHeading.Show()
		g.popup.questionCard.Show()
	} else {
		g.popup.requestHeading.Hide()
		g.popup.questionCard.Hide()
	}

	status := g.state.status
	if g.state.errorText != "" {
		status = g.state.errorText
	}
	g.popup.setStatus(status, g.state.isRunning, g.state.spinnerFrame)

	if g.state.isRunning {
		g.popup.actionButton.SetText("Cancel")
		g.popup.actionButton.Enable()
	} else {
		g.popup.actionButton.SetText("Submit")
		g.popup.actionButton.Enable()
	}

	if g.state.output != "" {
		g.popup.copyButton.Enable()
	} else {
		g.popup.copyButton.Disable()
	}

}

func (g *App) openSettings() {
	g.popup.showSettingsDialog(settingsDialogOptions{
		InitialProvider: g.cfg.GetDefaultProvider(),
		InitialModel:    g.cfg.GetDefaultModelId(),
		Version:         g.version,
		EnvResult:       g.envResult,
		ModelForProvider: func(provider string) string {
			return g.cfg.GetModelIdForProvider(provider)
		},
		OnSave: func(provider, model string) error {
			provider = strings.TrimSpace(provider)
			model = strings.TrimSpace(model)
			if err := validateProviderModel(provider, model); err != nil {
				return err
			}
			if err := g.cfg.SetDefaultProvider(provider); err != nil {
				return err
			}
			if err := g.cfg.SetDefaultModelId(model); err != nil {
				return err
			}
			g.state.status = "Settings saved."
			g.state.errorText = ""
			g.render()
			return nil
		},
	})
}

// openTestMenu shows the dev-only test dialog. Each entry injects a canned
// response so rendering can be exercised without a live model call.
func (g *App) openTestMenu() {
	g.popup.showTestDialog([]devTest{
		{
			name: "Exhaustive markdown",
			run: func() {
				g.showCannedOutput("Markdown feature test", exhaustiveMarkdown)
			},
		},
	})
}

// showCannedOutput places static content in the response area as if a run had
// completed, without contacting any provider.
func (g *App) showCannedOutput(question, output string) {
	if g.state.isRunning {
		g.cancel()
	}
	g.state.resetOutput()
	g.state.question = question
	g.state.output = output
	g.state.showRequest = true
	g.renderOutput()
	g.render()
}

// renderOutput pushes the current response into the view.
func (g *App) renderOutput() {
	g.popup.setOutput(g.state.output)
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
