package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/laszukdawid/terminal-agent/internal/voice"
)

const (
	defaultWindowWidth  float32 = 860
	defaultWindowHeight float32 = 600
	minWindowHeight     float32 = 520
	maxWindowHeight     float32 = 900
	compactOutputHeight float32 = 150
	sidebarWidth        float32 = 208
	navIconSize         float32 = 16
	micRingSize         float32 = 54
	statusIndicatorSize float32 = 18

	wordmarkText        = "TERMINAL AGENT"
	promptGlyph         = ">_"
	inputPrompt         = ">"
	sectionAsk          = "ASK THE TERMINAL AGENT"
	sectionTask         = "TASK THE TERMINAL AGENT TO DO"
	sectionResp         = "RESPONSE"
	sectionErr          = "ERROR"
	sendButtonText      = "SEND  ›"
	stopButtonText      = "STOP  ■"
	autoApproveHintText = "Auto Approve"

	// Listen control state words. The visible caption applies letter spacing
	// (see spaced); the raw words are shared with tests.
	listenWordIdle      = "LISTEN"
	listenWordRecording = "LISTENING"
	listenWordWorking   = "WORKING"
	listenWordOff       = "MIC OFF"
	listenWordBusy      = "BUSY"
)

var spinnerFrames = []string{"|", "/", "-", "\\"}

type popupWindow struct {
	window fyne.Window

	input          *popupEntry
	actionButton   *widget.Button
	actionSubtitle *canvas.Text

	inputHeading    *canvas.Text
	navAsk          *navRow
	navTask         *navRow
	responseHeading *canvas.Text
	responseSection *fyne.Container
	outputField     *widget.RichText
	transcriptBody  *fyne.Container
	outputBody      fyne.CanvasObject
	errorLabel      *widget.Label
	errorBody       fyne.CanvasObject
	outputScroll    *container.Scroll
	metaLabel       *canvas.Text
	lastRendered    string
	lastError       string
	transcriptViews []transcriptBlockView

	headerStatus    *widget.Label
	headerBrain     *canvas.Text
	headerSpinner   *canvas.Text
	headerStatusBox fyne.CanvasObject
	modelLabel      *widget.Label

	listenButton   *widget.Button
	listenLabel    *fyne.Container
	listenWord     string
	listenSubtitle *canvas.Text
	micRing        *canvas.Circle

	copyButton   *widget.Button
	exportButton *widget.Button

	mascotImage *canvas.Image
	robotIdle   fyne.Resource
	robotFrames []fyne.Resource
	mascotFrame int
	dataLane    *dataLane
	host        *hostNode
	mascotAnim  *fyne.Animation

	cwdLabel *marqueeLabel

	testButton *navRow

	onSubmit      func()
	onEscape      func()
	onAction      func()
	onVoiceToggle func()
	onCopy        func()
	onExport      func()
	onSettings    func()
	onSelectAsk   func()
	onSelectTask  func()
	onTest        func()
	onInput       func(string)
	onQuit        func()
}

func newPopupWindow(app fyne.App, devMode bool) *popupWindow {
	win := app.NewWindow("Terminal Agent")
	win.Resize(fyne.NewSize(defaultWindowWidth, defaultWindowHeight))
	win.SetFixedSize(false)
	win.CenterOnScreen()

	p := &popupWindow{window: win}

	p.buildInput(app)
	p.buildOutput()
	p.buildStatus()

	sidebar := p.buildSidebar(devMode)
	workspace := p.buildWorkspace()

	body := container.NewBorder(
		nil,
		nil,
		container.NewBorder(nil, nil, nil, brandVSeparator(), sidebar),
		nil,
		workspace,
	)
	win.SetContent(container.NewPadded(body))

	p.wireInteractions(app)
	return p
}

// buildInput creates the terminal-style prompt entry and the in-field SEND
// action. SEND lives inside the prompt panel so it is unambiguous which text
// is being sent (per brand guidance); there is no separate bottom submit.
func (p *popupWindow) buildInput(app fyne.App) {
	input := newPopupEntry(app)
	input.SetPlaceHolder("what model is this?")
	input.SetMinRowsVisible(1)
	p.input = input

	action := widget.NewButton(sendButtonText, nil)
	action.Importance = widget.HighImportance
	p.actionButton = action
}

func (p *popupWindow) buildOutput() {
	outputField := widget.NewRichText()
	outputField.Wrapping = fyne.TextWrapWord
	p.outputField = outputField

	transcriptBody := container.NewVBox()
	transcriptBody.Hide()
	p.transcriptBody = transcriptBody
	p.outputBody = container.NewStack(outputField, transcriptBody)

	errorLabel := widget.NewLabel("")
	errorLabel.Wrapping = fyne.TextWrapBreak
	errorLabel.Importance = widget.DangerImportance
	p.errorLabel = errorLabel
	p.errorBody = errorLabel
	p.errorBody.Hide()

	outputStack := container.NewStack(p.outputBody, p.errorBody)
	scroll := container.NewVScroll(outputStack)
	scroll.SetMinSize(fyne.NewSize(0, compactOutputHeight))
	p.outputScroll = scroll

	p.metaLabel = canvas.NewText("", brandSecondaryText)
	p.metaLabel.TextSize = theme.TextSize() - 2
}

func (p *popupWindow) buildStatus() {
	headerStatus := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	headerStatus.Importance = widget.MediumImportance
	headerStatus.Wrapping = fyne.TextWrapOff
	p.headerStatus = headerStatus

	brain := canvas.NewText("🧠", brandAccentGreen)
	brain.Alignment = fyne.TextAlignCenter
	brain.TextSize = theme.TextSize() + 2
	brain.Hide()
	p.headerBrain = brain

	spinner := canvas.NewText(spinnerFrames[0], brandAccentGreen)
	spinner.Alignment = fyne.TextAlignCenter
	spinner.TextSize = theme.TextSize() + 4
	spinner.Hide()
	p.headerSpinner = spinner

	statusSlot := container.NewGridWrap(
		fyne.NewSize(140, max(headerStatus.MinSize().Height, statusIndicatorSize)),
		container.NewCenter(headerStatus),
	)
	p.headerStatusBox = container.NewStack(statusSlot, container.NewCenter(brain), container.NewCenter(spinner))
}

func (p *popupWindow) buildWordmark() fyne.CanvasObject {
	mark := canvas.NewText(promptGlyph, brandAccentGreen)
	mark.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	mark.TextSize = theme.TextSize() + 2

	word := canvas.NewText(wordmarkText, brandPrimaryText)
	word.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	word.TextSize = theme.TextSize() - 1

	return container.NewHBox(mark, word)
}

func (p *popupWindow) buildModelPill() fyne.CanvasObject {
	modelDot := newStatusDot(brandAccentGreen)
	p.modelLabel = widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true, Monospace: true})
	return container.NewHBox(modelDot, p.modelLabel)
}

func (p *popupWindow) buildSidebar(devMode bool) fyne.CanvasObject {
	// ASK and TASK are the two live modes. ASK keeps the chat-bubble icon from
	// the original single-mode sidebar; TASK gets the action "zap" glyph. The
	// rows own only their styling and fire selection callbacks; App decides what
	// Ask/Task mean and owns the mode state.
	p.navAsk = newNavRow("ASK", iconPathChat, true, func() {
		if p.onSelectAsk != nil {
			p.onSelectAsk()
		}
	})
	p.navTask = newNavRow("TASK", iconPathTask, false, func() {
		if p.onSelectTask != nil {
			p.onSelectTask()
		}
	})
	nav := container.NewVBox(p.navAsk, p.navTask)
	// HISTORY, ENV and TOOLS are not wired up yet, so they are only shown in
	// dev mode where we can build them out without exposing dead nav entries.
	if devMode {
		nav.Add(newNavRow("HISTORY", iconPathHistory, false, nil))
		nav.Add(newNavRow("ENV", iconPathEnv, false, nil))
		nav.Add(newNavRow("TOOLS", iconPathTools, false, nil))
	}
	nav.Add(newNavRow("SETTINGS", iconPathSettings, false, func() {
		if p.onSettings != nil {
			p.onSettings()
		}
	}))

	if devMode {
		nav.Add(brandSeparator())
		p.testButton = newNavRow("TEST", iconPathTest, false, func() {
			if p.onTest != nil {
				p.onTest()
			}
		})
		nav.Add(p.testButton)
	}

	// The listen toggle and connection status share one bordered panel, divided
	// by a thin rule, mirroring the brand's lower-sidebar status card.
	listenPanel := borderedBox(container.NewVBox(
		p.buildListenControl(),
		brandSeparator(),
		p.buildConnectionStatus(),
	), brandBorder)

	content := container.NewVBox(
		p.buildWordmark(),
		brandSeparator(),
		nav,
		layout.NewSpacer(),
		listenPanel,
	)

	bg := canvas.NewRectangle(brandPanel)
	padded := container.New(layout.NewCustomPaddedLayout(panelPadV, panelPadV, panelPadH, panelPadH), content)
	panel := container.NewStack(bg, padded)
	return container.New(&fixedWidthLayout{width: sidebarWidth}, panel)
}

func (p *popupWindow) buildListenControl() fyne.CanvasObject {
	ring := canvas.NewCircle(color.Transparent)
	ring.StrokeColor = brandAccentGreen
	ring.StrokeWidth = 1.5
	p.micRing = ring

	// The mic ring is itself the toggle button: an icon-only, borderless button
	// sized to the ring, with the ring stroke drawn over it.
	p.listenButton = widget.NewButtonWithIcon("", lineIcon("mic", iconPathMic, brandAccentGreen), nil)
	p.listenButton.Importance = widget.LowImportance
	mic := container.NewGridWrap(
		fyne.NewSize(micRingSize, micRingSize),
		container.NewStack(p.listenButton, ring),
	)

	// LISTEN is a caption (not the button label) so it can be letter-spaced and
	// sit tight against the subtitle below it.
	p.listenLabel = letterRow(listenWordIdle, brandPrimaryText, theme.TextSize()+1)
	p.listenWord = listenWordIdle

	p.listenSubtitle = canvas.NewText("Press to toggle mic", brandSecondaryText)
	p.listenSubtitle.Alignment = fyne.TextAlignCenter
	p.listenSubtitle.TextSize = theme.TextSize() - 2

	caption := container.NewVBox(
		container.NewCenter(p.listenLabel),
		container.NewCenter(p.listenSubtitle),
	)

	return container.NewVBox(
		container.NewCenter(mic),
		caption,
	)
}

func (p *popupWindow) buildConnectionStatus() fyne.CanvasObject {
	p.cwdLabel = newMarqueeLabel("~/", brandMutedGreen, theme.TextSize()-1)

	return container.NewVBox(p.cwdLabel)
}

func (p *popupWindow) buildWorkspace() fyne.CanvasObject {
	p.inputHeading = brandSectionLabel(sectionAsk)
	headingRow := container.NewBorder(
		nil, nil,
		p.inputHeading,
		container.NewHBox(p.headerStatusBox, hStrut(12), p.buildModelPill()),
		nil,
	)

	prompt := canvas.NewText(inputPrompt, brandAccentGreen)
	prompt.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	prompt.TextSize = theme.TextSize() + 3

	p.actionSubtitle = canvas.NewText("", brandSecondaryText)
	p.actionSubtitle.Alignment = fyne.TextAlignCenter
	p.actionSubtitle.TextStyle = fyne.TextStyle{Monospace: true}
	p.actionSubtitle.TextSize = theme.TextSize() - 4
	p.actionSubtitle.Hide()
	inputWithNativeCursor := container.NewThemeOverride(p.input, promptEntryTheme{Theme: newBrandTheme()})
	actionControl := container.NewVBox(p.actionButton, p.actionSubtitle)

	inputRow := container.NewBorder(
		nil, nil,
		vCenter(prompt),
		vCenter(actionControl),
		vCenter(inputWithNativeCursor),
	)
	inputPanel := borderedBox(inputRow, brandBorderBright)

	p.responseHeading = brandSectionLabel(sectionResp)

	metaRow := container.NewVBox(brandSeparator(), p.metaLabel)
	responsePanelInner := container.NewBorder(nil, metaRow, nil, nil, p.outputScroll)
	responsePanel := borderedBox(responsePanelInner, brandBorder)

	p.copyButton = newCommandButton("COPY", iconPathCopy, func() {
		if p.onCopy != nil {
			p.onCopy()
		}
	})
	p.copyButton.Disable()
	p.exportButton = newCommandButton("EXPORT", iconPathExport, func() {
		if p.onExport != nil {
			p.onExport()
		}
	})
	p.exportButton.Disable()
	outputActions := container.NewHBox(p.copyButton, brandActionDivider(), p.exportButton)

	p.responseSection = container.NewBorder(
		container.NewVBox(p.responseHeading),
		outputActions,
		nil, nil,
		responsePanel,
	)
	p.responseSection.Hide()

	askGroup := container.NewVBox(headingRow, inputPanel)

	workspace := container.NewBorder(
		askGroup,
		p.buildMascotPanel(),
		nil, nil,
		p.responseSection,
	)
	// Inset the whole workspace from the sidebar divider and window edges so the
	// section headings and panels are not flush against the border.
	return container.New(layout.NewCustomPaddedLayout(panelPadV, panelPadV, panelPadH, panelPadH), workspace)
}

func (p *popupWindow) wireInteractions(app fyne.App) {
	win := p.window
	p.cwdLabel.SetOnCopy(func(text string) {
		app.Clipboard().SetContent(text)
	})

	p.input.onEscape = func() {
		if p.onEscape != nil {
			p.onEscape()
		}
	}
	p.input.onSubmit = func() {
		if p.onSubmit != nil {
			p.onSubmit()
		}
	}
	p.input.onVoiceToggle = func() {
		if p.onVoiceToggle != nil {
			p.onVoiceToggle()
		}
	}
	p.input.OnChanged = func(value string) {
		p.resizeInput(value)
		if p.onInput != nil {
			p.onInput(value)
		}
	}
	p.actionButton.OnTapped = func() {
		if p.onAction != nil {
			p.onAction()
		}
		win.Canvas().Focus(p.input)
	}
	p.listenButton.OnTapped = func() {
		if p.onVoiceToggle != nil {
			p.onVoiceToggle()
		}
		win.Canvas().Focus(p.input)
	}

	win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyL, Modifier: fyne.KeyModifierShortcutDefault}, func(fyne.Shortcut) {
		win.Canvas().Focus(p.input)
	})
	win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyQ, Modifier: fyne.KeyModifierShortcutDefault}, func(fyne.Shortcut) {
		if p.onQuit != nil {
			p.onQuit()
		}
	})
}

func (p *popupWindow) setListenButton(enabled bool, state voice.State) {
	switch state {
	case voice.StateRecording:
		p.setListenWord(listenWordRecording)
		p.listenSubtitle.Text = "Press to stop"
		p.micRing.StrokeColor = brandAccentGreen
	case voice.StateTranscribing:
		p.setListenWord(listenWordWorking)
		p.listenSubtitle.Text = "Transcribing…"
		p.micRing.StrokeColor = brandMutedGreen
	default:
		if enabled {
			p.setListenWord(listenWordIdle)
			p.listenSubtitle.Text = "Press to toggle mic"
			p.micRing.StrokeColor = brandAccentGreen
		} else {
			p.setListenWord(listenWordOff)
			p.listenSubtitle.Text = "Check settings"
			p.micRing.StrokeColor = brandBorder
		}
	}
	p.micRing.Refresh()
	p.listenSubtitle.Refresh()
	if enabled {
		p.listenButton.Enable()
	} else {
		p.listenButton.Disable()
	}
}

// setListenWord sets the letter-spaced LISTEN caption to word.
func (p *popupWindow) setListenWord(word string) {
	if p.listenWord == word {
		return
	}
	p.listenWord = word
	p.listenLabel.Objects = letterRow(word, brandPrimaryText, theme.TextSize()+1).Objects
	p.listenLabel.Refresh()
}

func (p *popupWindow) setMeta(text string) {
	if p.metaLabel.Text == text {
		return
	}
	p.metaLabel.Text = text
	p.metaLabel.Refresh()
}

// setMode updates the sidebar active row and the input section heading to match
// the selected mode, without rebuilding the workspace or window.
func (p *popupWindow) setMode(mode guiMode) {
	if p.navAsk != nil {
		p.navAsk.setActive(mode == guiModeAsk)
	}
	if p.navTask != nil {
		p.navTask.setActive(mode == guiModeTask)
	}
	p.setInputHeading(inputHeadingForMode(mode))
}

func (p *popupWindow) setInputHeading(text string) {
	if p.inputHeading == nil || p.inputHeading.Text == text {
		return
	}
	p.inputHeading.Text = text
	p.inputHeading.Refresh()
}

func (p *popupWindow) setActionSubtitle(text string) {
	if p.actionSubtitle == nil || p.actionSubtitle.Text == text {
		return
	}
	p.actionSubtitle.Text = text
	if text == "" {
		p.actionSubtitle.Hide()
	} else {
		p.actionSubtitle.Show()
	}
	p.actionSubtitle.Refresh()
}

func (p *popupWindow) setResponseHeading(text string) {
	if p.responseHeading.Text == text {
		return
	}
	p.responseHeading.Text = text
	p.responseHeading.Refresh()
}

func (p *popupWindow) setCwd(text string) {
	if p.cwdLabel.Text() == text {
		return
	}
	p.cwdLabel.SetText(text)
}

func (p *popupWindow) setOutput(content string) {
	p.errorBody.Hide()
	p.outputBody.Show()
	p.outputField.Show()
	p.transcriptBody.Hide()
	if p.lastRendered == content {
		return
	}
	p.lastRendered = content
	p.outputField.ParseMarkdown(decorateDollarMarkers(unwrapMarkdownFence(content)))
	p.outputField.Segments = colorizeDollarMarkers(p.outputField.Segments)
	p.outputField.Refresh()
}

func (p *popupWindow) setError(content string) {
	p.outputBody.Hide()
	p.errorBody.Show()
	if p.lastError == content {
		return
	}
	p.lastError = content
	p.errorLabel.SetText(content)
}

func (p *popupWindow) setStatus(status string, isRunning bool, spinnerFrame int) {
	p.headerBrain.Color = brandAccentGreen
	p.headerBrain.Refresh()
	p.headerSpinner.Color = brandAccentGreen
	p.headerSpinner.Refresh()
	p.headerBrain.Hide()
	p.headerSpinner.Hide()
	p.headerStatus.Show()

	switch status {
	case "thinking":
		p.headerStatus.SetText("")
		p.headerStatus.Hide()
		p.headerBrain.Show()
	case "responding":
		if isRunning {
			p.headerStatus.SetText("")
			p.headerStatus.Hide()
			p.headerSpinner.Text = spinnerFrames[spinnerFrame%len(spinnerFrames)]
			p.headerSpinner.Refresh()
			p.headerSpinner.Show()
			return
		}
		p.headerStatus.SetText("")
	default:
		p.headerStatus.SetText(status)
		p.headerSpinner.Text = spinnerFrames[0]
		p.headerSpinner.Refresh()
	}
}

func max(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
