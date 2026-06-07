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
	"github.com/laszukdawid/terminal-agent/internal/voice"
)

type App struct {
	fyneApp         fyne.App
	service         appservice.Service
	cfg             config.Config
	state           *state
	popup           *popupWindow
	quit            func()
	stopIndicator   chan struct{}
	version         string
	envResult       EnvironmentLoadResult
	voiceController *voice.Controller
	voiceSchedule   func(func())
}

type AppOptions struct {
	AppID   string
	DevMode bool
	Version string
	Voice   VoiceOptions
	FyneApp fyne.App
}

type VoiceOptions struct {
	Recorder    voice.Recorder
	Transcriber voice.Transcriber
	Schedule    func(func())
}

func NewApp(service appservice.Service, cfg config.Config, options AppOptions) *App {
	fyneApp := options.FyneApp
	if fyneApp == nil {
		fyneApp = app.NewWithID(options.AppID)
	}
	gui := &App{
		fyneApp:       fyneApp,
		service:       service,
		cfg:           cfg,
		state:         &state{},
		quit:          fyneApp.Quit,
		stopIndicator: make(chan struct{}),
		version:       options.Version,
		voiceSchedule: fyne.Do,
	}
	if options.Voice.Schedule != nil {
		gui.voiceSchedule = options.Voice.Schedule
	}
	fyneApp.Settings().SetTheme(newBrandTheme())
	if icon, err := loadAppIcon(); err == nil {
		fyneApp.SetIcon(icon)
	}
	gui.popup = newPopupWindow(fyneApp, options.DevMode)
	gui.initVoice(options.Voice)
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
	g.cancelVoice()
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

func (g *App) Quit() {
	g.cancelVoice()
	if g.state.cancelFunc != nil {
		g.state.cancelFunc()
		g.state.clearRunning()
	}
	g.quit()
}

func (g *App) startIndicator() {
	g.popup.startMascotAnimation()
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
	g.popup.stopMascotAnimation()
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
	g.popup.onExport = g.exportOutput
	g.popup.onSettings = g.openSettings
	g.popup.onTest = g.openTestMenu
	g.popup.onVoiceToggle = g.toggleVoice
	g.popup.input.voiceTriggerKey = fyne.KeyName(g.cfg.GetGUIVoiceTriggerKey())
	g.popup.input.onVoiceToggle = g.toggleVoice
	g.popup.window.Canvas().SetOnTypedKey(func(key *fyne.KeyEvent) {
		if key.Name == fyne.KeyName(g.cfg.GetGUIVoiceTriggerKey()) {
			g.toggleVoice()
		}
	})
	g.popup.onEscape = func() {
		g.Hide()
	}
	g.popup.onQuit = g.Quit
	g.popup.onInput = func(value string) {
		g.state.input = value
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
				g.Quit()
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
	if !g.state.isRunning {
		g.renderResponse()
	}

	provider := g.cfg.GetDefaultProvider()
	model := g.cfg.GetDefaultModelId()
	g.popup.modelLabel.SetText("MODEL: " + provider + " / " + model)
	g.popup.setCwd(displayCwd(g.cfg.GetWorkingDir()))

	showAnswer := g.state.output != "" || g.state.isRunning || g.state.errorText != ""
	if showAnswer {
		g.popup.responseSection.Show()
	} else {
		g.popup.responseSection.Hide()
	}
	if g.state.errorText != "" {
		g.popup.setResponseHeading(sectionErr)
	} else {
		g.popup.setResponseHeading(sectionResp)
	}
	g.popup.setMeta(metaText(g.state))

	g.popup.setStatus(displayStatus(g.state), g.state.isRunning, g.state.spinnerFrame)

	if g.state.isRunning {
		g.popup.actionButton.SetText(stopButtonText)
	} else {
		g.popup.actionButton.SetText(sendButtonText)
	}
	g.popup.actionButton.Enable()

	voiceEnabled := g.cfg.GetGUIVoiceEnabled() && g.voiceController != nil
	voiceBlockedByRunning := g.state.isRunning && g.state.voiceState == voice.StateIdle
	if voiceBlockedByRunning {
		voiceEnabled = false
	}
	g.popup.setListenButton(voiceEnabled, g.state.voiceState)
	if voiceBlockedByRunning {
		g.popup.setListenWord(listenWordBusy)
	}

	if hasCopyableResponse(g.state) {
		g.popup.copyButton.Enable()
		g.popup.exportButton.Enable()
	} else {
		g.popup.copyButton.Disable()
		g.popup.exportButton.Disable()
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
		OnClosed: g.FocusInput,
	})
}

// renderResponse pushes the current response or error into the view.
func (g *App) renderResponse() {
	if g.state.errorText != "" {
		g.popup.setError(g.state.errorText)
		return
	}
	g.popup.setOutput(g.state.output)
}

// renderOutput pushes the current response into the view.
func (g *App) renderOutput() {
	g.popup.setOutput(g.state.output)
}

func displayStatus(s *state) string {
	if s.errorText != "" {
		return "Error"
	}
	return s.status
}

const (
	clockFormat   = "15:04:05"
	metaSeparator = "    |    "
)

// metaText builds the observable-execution metadata row shown at the bottom of
// the response panel: runtime and completion timestamp. It reports "running…"
// while a response streams and stays empty until there is a completed response
// so the row never shows stale or fabricated numbers.
func metaText(s *state) string {
	if s.isRunning {
		return "running…"
	}
	if s.errorText != "" || s.output == "" {
		return ""
	}
	parts := make([]string, 0, 2)
	if s.elapsed > 0 {
		parts = append(parts, "◷ "+formatElapsed(s.elapsed))
	}
	if !s.completedAt.IsZero() {
		parts = append(parts, s.completedAt.Format(clockFormat))
	}
	return strings.Join(parts, metaSeparator)
}

func formatElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// maxCwdChars bounds the working-directory label so it stays inside the
// sidebar status panel; longer paths are shown tail-first with a leading "…".
const maxCwdChars = 22

// displayCwd renders the working directory in terminal "~/"-relative form for
// the sidebar connection status, truncated to fit the status panel.
func displayCwd(dir string) string {
	if dir == "" {
		return "~/"
	}
	path := dir
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(dir, home) {
		rel := strings.TrimPrefix(dir, home)
		if rel == "" {
			return "~/"
		}
		path = "~" + rel
	}
	if len(path) > maxCwdChars {
		path = "…" + path[len(path)-(maxCwdChars-1):]
	}
	return path
}

func hasCopyableResponse(s *state) bool {
	return s.errorText != "" || s.output != ""
}

func responseCopyText(s *state) string {
	if s.errorText != "" {
		return s.errorText
	}
	if s.output != "" {
		return s.output
	}
	return s.question
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
