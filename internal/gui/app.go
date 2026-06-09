package gui

import (
	"context"
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
	pending         *pendingInteraction
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
		state:         &state{mode: guiModeAsk},
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
	// Hide/Quit are run-termination paths too: tear down any pending interaction
	// so a clarification dialog can never outlive the run (or the window).
	g.dismissInteraction()
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
	g.dismissInteraction()
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
					if !g.state.isRunning {
						return
					}
					// Coalesced output flush: Task streaming marks state dirty and
					// lets this ticker render at its cadence instead of re-parsing the
					// whole transcript on every chunk. Ask never sets outputDirty, so
					// this is a no-op for Ask.
					if g.state.outputDirty {
						g.state.outputDirty = false
						g.renderOutput()
						g.popup.outputScroll.ScrollToBottom()
					}
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
	g.popup.onSelectAsk = func() { g.setMode(guiModeAsk) }
	g.popup.onSelectTask = func() { g.setMode(guiModeTask) }
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

	showAnswer := g.state.hasResponseContent() || g.state.isRunning || g.state.errorText != ""
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

	if g.state.isRunning {
		g.popup.actionButton.SetText(stopButtonText)
		g.popup.setActionSubtitle("")
	} else {
		g.popup.actionButton.SetText(sendButtonText)
		if g.state.mode == guiModeTask {
			g.popup.setActionSubtitle(autoApproveHintText)
		} else {
			g.popup.setActionSubtitle("")
		}
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
	if g.state.mode == guiModeTask {
		g.popup.setTranscript(g.state.taskTranscript)
		return
	}
	g.popup.setOutput(g.state.output)
}

// renderOutput pushes the current response into the view.
func (g *App) renderOutput() {
	g.renderResponse()
}

// setMode switches the active sidebar tab. Switching is ignored while a request
// is in flight to avoid ambiguous ownership of the running stream.
func (g *App) setMode(mode guiMode) {
	if g.state.isRunning || g.state.mode == mode {
		return
	}
	g.state.saveModeView()
	g.state.restoreModeView(mode)
	g.popup.setMode(mode)
	g.render()
}

// beginRun performs the shared setup both modes need before dispatching to the
// service, and returns the cancellable context for the run.
func (g *App) beginRun(message string) context.Context {
	g.state.resetOutput()
	g.state.question = message
	ctx, cancel := context.WithCancel(context.Background())
	g.state.setRunning(cancel)
	g.renderOutput()
	g.startIndicator()
	g.render()
	return ctx
}

// failRunSetup unwinds an in-flight run when the service rejects the request
// before any events are produced.
func (g *App) failRunSetup(err error) {
	g.stopIndicatorAnimation()
	g.state.clearRunning()
	g.state.errorText = runtimeErrorMessage(err)
	g.render()
}

// finishRun is the single chokepoint for run termination, shared by Ask and
// Task. It dismisses any pending interaction, records timing, stops the
// indicator, applies cancel/error display, clears running state, and refocuses
// input. A nil err means successful completion.
func (g *App) finishRun(err error) {
	g.dismissInteraction()
	// finishRun owns the final output flush: any coalesced (dirty) transcript
	// content is rendered here so terminal callers never have to remember to.
	if g.state.outputDirty {
		g.state.outputDirty = false
		g.renderOutput()
	}
	g.state.markCompleted()
	g.stopIndicatorAnimation()
	if err != nil {
		if isCanceledError(err) {
			g.state.errorText = ""
			g.state.status = "Canceled."
		} else {
			g.state.errorText = runtimeErrorMessage(err)
		}
	}
	g.state.clearRunning()
	g.FocusInput()
}

// taskAutoApprove reports whether GUI Task runs auto-approve actions. Today it is
// always true; permission prompts are a future release, at which point this
// becomes config/UI-driven and EventConfirmationNeeded graduates from a backstop
// into a real confirmation dialog routed through the interaction controller.
func (g *App) taskAutoApprove() bool {
	return true
}

// inputHeadingForMode returns the input section heading for the active mode.
func inputHeadingForMode(mode guiMode) string {
	if mode == guiModeTask {
		return sectionTask
	}
	return sectionAsk
}

// emptyInputMessage returns the validation message shown for an empty submit.
func emptyInputMessage(mode guiMode) string {
	if mode == guiModeTask {
		return "Task cannot be empty."
	}
	return "Question cannot be empty."
}

// exportPromptHeadingForMode returns the request section heading used in the
// exported markdown ("# Ask" / "# Task").
func exportPromptHeadingForMode(mode guiMode) string {
	if mode == guiModeTask {
		return "Task"
	}
	return "Ask"
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
	if s.errorText != "" || !s.hasResponseContent() {
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

// displayCwd renders the working directory in terminal "~/"-relative form for
// the sidebar connection status. The sidebar widget owns width-constrained
// rendering so it can scroll long paths instead of truncating them.
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
	return path
}

func hasCopyableResponse(s *state) bool {
	if s.isRunning {
		return false
	}
	return s.errorText != "" || s.hasResponseContent()
}

func responseCopyText(s *state) string {
	if s.errorText != "" {
		return s.errorText
	}
	if s.hasResponseContent() {
		return s.responseText()
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
