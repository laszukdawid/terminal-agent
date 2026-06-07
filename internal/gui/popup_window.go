package gui

import (
	"image/color"
	"math"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/voice"
)

const (
	defaultWindowWidth  float32 = 860
	defaultWindowHeight float32 = 600
	minWindowHeight     float32 = 520
	maxWindowHeight     float32 = 900
	maxInputRows                = 3
	compactOutputHeight float32 = 150
	sidebarWidth        float32 = 208
	navIconSize         float32 = 16
	micRingSize         float32 = 54
	mascotSize          float32 = 76
	statusIndicatorSize float32 = 18

	providerStatusWidth  float32 = 28
	providerStatusHeight float32 = 24
	promptCursorWidth    float32 = 3
	promptCursorMinAlpha uint8   = 0x22
	promptCursorDuration         = 650 * time.Millisecond
	inputTextInset       float32 = 24

	// Mascot "walking" wiggle and the data-transfer dots, played while a request
	// is in flight.
	mascotFrameCount   = 12
	mascotAntennaSwing = 2.4
	mascotLegSwing     = 3.6
	mascotAnimDuration = 1300 * time.Millisecond
	mascotDotCount     = 20

	// Host receive reaction: when the packet's position (the triangle wave)
	// crosses hostReceiveStart, the host begins reacting; the shake oscillates
	// at hostShakeFreq cycles across the animation loop.
	hostReceiveStart float32 = 0.7
	hostShakeFreq            = 9.0

	wordmarkText   = "TERMINAL AGENT"
	promptGlyph    = ">_"
	inputPrompt    = ">"
	sectionAsk     = "ASK THE TERMINAL AGENT"
	sectionTask    = "TASK THE TERMINAL AGENT TO DO"
	sectionResp    = "RESPONSE"
	sectionErr     = "ERROR"
	sendButtonText = "SEND  ›"
	stopButtonText = "STOP  ■"
	sendHintText   = "Enter to send"
	tagline1       = "Your terminal."
	tagline2       = "My context."

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

	input        *popupEntry
	actionButton *widget.Button

	inputHeading    *canvas.Text
	navAsk          *navRow
	navTask         *navRow
	responseHeading *canvas.Text
	responseSection *fyne.Container
	outputField     *widget.RichText
	outputBody      fyne.CanvasObject
	errorLabel      *widget.Label
	errorBody       fyne.CanvasObject
	outputScroll    *container.Scroll
	metaLabel       *canvas.Text
	lastRendered    string
	lastError       string

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

	cwdLabel *canvas.Text

	testButton *widget.Button

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

type popupEntry struct {
	widget.Entry
	app             fyne.App
	onEscape        func()
	onSubmit        func()
	onVoiceToggle   func()
	onFocusChanged  func(bool)
	voiceTriggerKey fyne.KeyName
	focusCursor     *canvas.Rectangle
	cursorAnim      *fyne.Animation
	visibleTopRow   int
}

type settingsTextEntry struct {
	widget.Entry
	onFocusChanged func(bool)
	focusCursor    *canvas.Rectangle
	cursorAnim     *fyne.Animation
}

type providerStatusIcon struct {
	widget.BaseWidget
	canvas  fyne.Canvas
	icon    *canvas.Text
	message string
	popup   *widget.PopUp
}

type settingsDialogOptions struct {
	InitialProvider  string
	InitialModel     string
	Version          string
	EnvResult        EnvironmentLoadResult
	ModelForProvider func(provider string) string
	OnSave           func(provider, model string) error
	OnClosed         func()
}

func newProviderStatusIcon(c fyne.Canvas) *providerStatusIcon {
	status := &providerStatusIcon{
		canvas: c,
		icon:   canvas.NewText("", theme.Color(theme.ColorNameForeground)),
	}
	status.icon.TextSize = theme.TextSize() + 1
	status.icon.TextStyle = fyne.TextStyle{Bold: true}
	status.ExtendBaseWidget(status)
	return status
}

func (s *providerStatusIcon) setStatus(status providerReadiness) {
	s.message = ""
	if status.Message == "" {
		s.icon.Text = ""
	} else if status.Available {
		s.icon.Text = "✓"
		s.icon.Color = theme.Color(theme.ColorNameSuccess)
	} else {
		s.icon.Text = "✕"
		s.icon.Color = theme.Color(theme.ColorNameError)
		s.message = status.Message
	}
	if s.popup != nil {
		s.popup.Hide()
		s.popup = nil
	}
	s.icon.Refresh()
	s.Refresh()
}

func (s *providerStatusIcon) CreateRenderer() fyne.WidgetRenderer {
	return &providerStatusIconRenderer{icon: s.icon}
}

type providerStatusIconRenderer struct {
	icon *canvas.Text
}

func (r *providerStatusIconRenderer) Destroy() {}

func (r *providerStatusIconRenderer) Layout(size fyne.Size) {
	iconSize := r.icon.MinSize()
	r.icon.Resize(iconSize)
	r.icon.Move(fyne.NewPos((size.Width-iconSize.Width)/2, (size.Height-iconSize.Height)/2))
}

func (r *providerStatusIconRenderer) MinSize() fyne.Size {
	return fyne.NewSize(providerStatusWidth, providerStatusHeight)
}

func (r *providerStatusIconRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.icon}
}

func (r *providerStatusIconRenderer) Refresh() {
	r.icon.Refresh()
}

func (s *providerStatusIcon) MouseIn(e *desktop.MouseEvent) {
	if s.message == "" || s.canvas == nil {
		return
	}
	label := widget.NewLabel(s.message)
	label.Wrapping = fyne.TextWrapWord
	content := container.NewPadded(label)
	s.popup = widget.NewPopUp(content, s.canvas)
	s.popup.Resize(fyne.NewSize(360, content.MinSize().Height))
	pos := s.Position().AddXY(s.Size().Width+8, 0)
	if e != nil {
		pos = e.AbsolutePosition.AddXY(12, 12)
	}
	s.popup.ShowAtPosition(pos)
}

func (s *providerStatusIcon) MouseMoved(*desktop.MouseEvent) {}

func (s *providerStatusIcon) MouseOut() {
	if s.popup != nil {
		s.popup.Hide()
		s.popup = nil
	}
}

func newPopupEntry(app fyne.App) *popupEntry {
	entry := &popupEntry{app: app}
	entry.ExtendBaseWidget(entry)
	entry.MultiLine = true
	entry.Wrapping = fyne.TextWrapWord
	entry.Scroll = fyne.ScrollVerticalOnly
	return entry
}

func newSettingsTextEntry(initial string) *settingsTextEntry {
	entry := &settingsTextEntry{}
	entry.ExtendBaseWidget(entry)
	entry.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)
	entry.SetText(initial)
	return entry
}

func (e *settingsTextEntry) FocusGained() {
	e.Entry.FocusGained()
	if e.onFocusChanged != nil {
		e.onFocusChanged(true)
	}
}

func (e *settingsTextEntry) FocusLost() {
	e.Entry.FocusLost()
	if e.onFocusChanged != nil {
		e.onFocusChanged(false)
	}
}

func (e *settingsTextEntry) withFocusCursor() fyne.CanvasObject {
	cursor := canvas.NewRectangle(brandAccentGreen)
	cursor.Hide()
	e.focusCursor = cursor

	wrapped := container.New(&settingsCursorLayout{entry: e, cursor: cursor}, e, cursor)
	previousCursorChanged := e.OnCursorChanged
	e.OnCursorChanged = func() {
		if previousCursorChanged != nil {
			previousCursorChanged()
		}
		e.positionFocusCursor()
	}
	e.onFocusChanged = func(focused bool) {
		if focused {
			e.startFocusCursor()
		} else {
			e.stopFocusCursor()
		}
		e.positionFocusCursor()
	}
	return wrapped
}

func (e *settingsTextEntry) positionFocusCursor() {
	if e.focusCursor == nil {
		return
	}
	textSize := fyne.MeasureText("M", theme.TextSize(), e.TextStyle)
	e.focusCursor.Resize(fyne.NewSize(promptCursorWidth, textSize.Height))
	e.focusCursor.Move(e.CursorPosition())
	e.focusCursor.Refresh()
}

func (e *settingsTextEntry) startFocusCursor() {
	if e.focusCursor == nil || e.cursorAnim != nil {
		return
	}
	e.focusCursor.FillColor = brandAccentGreen
	e.focusCursor.Show()
	e.cursorAnim = fyne.NewAnimation(promptCursorDuration, func(progress float32) {
		alpha := promptCursorMinAlpha + uint8(float32(0xFF-promptCursorMinAlpha)*progress)
		e.focusCursor.FillColor = color.NRGBA{R: brandAccentGreen.R, G: brandAccentGreen.G, B: brandAccentGreen.B, A: alpha}
		e.focusCursor.Refresh()
	})
	e.cursorAnim.AutoReverse = true
	e.cursorAnim.RepeatCount = fyne.AnimationRepeatForever
	e.cursorAnim.Start()
}

func (e *settingsTextEntry) stopFocusCursor() {
	if e.cursorAnim != nil {
		e.cursorAnim.Stop()
		e.cursorAnim = nil
	}
	if e.focusCursor != nil {
		e.focusCursor.Hide()
	}
}

func (e *popupEntry) FocusGained() {
	e.Entry.FocusGained()
	if e.onFocusChanged != nil {
		e.onFocusChanged(true)
	}
}

func (e *popupEntry) FocusLost() {
	e.Entry.FocusLost()
	if e.onFocusChanged != nil {
		e.onFocusChanged(false)
	}
}

func (e *popupEntry) withFocusCursor() fyne.CanvasObject {
	cursor := canvas.NewRectangle(brandAccentGreen)
	cursor.Hide()
	e.focusCursor = cursor

	wrapped := container.New(&promptCursorLayout{entry: e, cursor: cursor}, e, cursor)
	previousCursorChanged := e.OnCursorChanged
	e.OnCursorChanged = func() {
		if previousCursorChanged != nil {
			previousCursorChanged()
		}
		e.positionFocusCursor()
	}
	e.onFocusChanged = func(focused bool) {
		if focused {
			e.startFocusCursor()
		} else {
			e.stopFocusCursor()
		}
		e.positionFocusCursor()
	}
	return wrapped
}

func (e *popupEntry) positionFocusCursor() {
	if e.focusCursor == nil {
		return
	}
	textSize := fyne.MeasureText("M", theme.TextSize(), e.TextStyle)
	e.focusCursor.Resize(fyne.NewSize(promptCursorWidth, textSize.Height))
	e.updateVisibleTopRow()
	pos := e.CursorPosition()
	pos.Y -= float32(e.visibleTopRow) * textSize.Height
	if pos.Y < 0 {
		pos.Y = 0
	}
	maxY := e.Size().Height - textSize.Height
	if maxY > 0 && pos.Y > maxY {
		pos.Y = maxY
	}
	e.focusCursor.Move(pos)
	e.focusCursor.Refresh()
}

func (e *popupEntry) updateVisibleTopRow() {
	rowCount := promptInputRowCount(e.Text, e.Size().Width)
	if cursorRows := e.CursorRow + 1; cursorRows > rowCount {
		rowCount = cursorRows
	}
	maxTopRow := rowCount - maxInputRows
	if maxTopRow < 0 {
		maxTopRow = 0
	}
	if e.visibleTopRow > maxTopRow {
		e.visibleTopRow = maxTopRow
	}
	if e.CursorRow < e.visibleTopRow {
		e.visibleTopRow = e.CursorRow
	}
	if e.CursorRow >= e.visibleTopRow+maxInputRows {
		e.visibleTopRow = e.CursorRow - maxInputRows + 1
	}
	if e.visibleTopRow < 0 {
		e.visibleTopRow = 0
	}
}

func (e *popupEntry) startFocusCursor() {
	if e.focusCursor == nil || e.cursorAnim != nil {
		return
	}
	e.focusCursor.FillColor = brandAccentGreen
	e.focusCursor.Show()
	e.cursorAnim = fyne.NewAnimation(promptCursorDuration, func(progress float32) {
		alpha := promptCursorMinAlpha + uint8(float32(0xFF-promptCursorMinAlpha)*progress)
		e.focusCursor.FillColor = color.NRGBA{R: brandAccentGreen.R, G: brandAccentGreen.G, B: brandAccentGreen.B, A: alpha}
		e.focusCursor.Refresh()
	})
	e.cursorAnim.AutoReverse = true
	e.cursorAnim.RepeatCount = fyne.AnimationRepeatForever
	e.cursorAnim.Start()
}

func (e *popupEntry) stopFocusCursor() {
	if e.cursorAnim != nil {
		e.cursorAnim.Stop()
		e.cursorAnim = nil
	}
	if e.focusCursor != nil {
		e.focusCursor.Hide()
	}
}

func (e *popupEntry) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case e.voiceTriggerKey:
		if e.onVoiceToggle != nil {
			e.onVoiceToggle()
		}
		return
	case fyne.KeyEscape:
		if e.onEscape != nil {
			e.onEscape()
		}
		return
	case fyne.KeyReturn, fyne.KeyEnter:
		if driver, ok := e.app.Driver().(desktop.Driver); ok {
			if driver.CurrentKeyModifiers()&fyne.KeyModifierShift != 0 {
				e.Entry.TypedKey(key)
				return
			}
		}
		if e.onSubmit != nil {
			e.onSubmit()
		}
		return
	}
	e.Entry.TypedKey(key)
}

type promptCursorLayout struct {
	entry  *popupEntry
	cursor *canvas.Rectangle
}

func (l *promptCursorLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return l.entry.MinSize()
}

func (l *promptCursorLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	l.entry.Resize(size)
	l.entry.Move(fyne.NewPos(0, 0))
	l.entry.positionFocusCursor()
}

type settingsCursorLayout struct {
	entry  *settingsTextEntry
	cursor *canvas.Rectangle
}

func (l *settingsCursorLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return l.entry.MinSize()
}

func (l *settingsCursorLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	l.entry.Resize(size)
	l.entry.Move(fyne.NewPos(0, 0))
	l.entry.positionFocusCursor()
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

	topBar := p.buildTopBar()
	sidebar := p.buildSidebar(devMode)
	workspace := p.buildWorkspace()

	body := container.NewBorder(
		container.NewVBox(topBar, brandSeparator()),
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
	p.outputBody = outputField

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

func (p *popupWindow) buildTopBar() fyne.CanvasObject {
	mark := canvas.NewText(promptGlyph, brandAccentGreen)
	mark.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	mark.TextSize = theme.TextSize() + 4

	word := canvas.NewText(wordmarkText, brandPrimaryText)
	word.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	word.TextSize = theme.TextSize() + 1

	wordmark := container.NewHBox(mark, word)

	modelDot := newStatusDot(brandAccentGreen)
	p.modelLabel = widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true, Monospace: true})
	modelPill := container.NewHBox(modelDot, p.modelLabel)

	return container.NewHBox(
		wordmark,
		layout.NewSpacer(),
		p.headerStatusBox,
		layout.NewSpacer(),
		modelPill,
	)
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
		p.testButton = widget.NewButton("Test", func() {
			if p.onTest != nil {
				p.onTest()
			}
		})
		p.testButton.Importance = widget.LowImportance
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
	p.cwdLabel = canvas.NewText("~/", brandMutedGreen)
	p.cwdLabel.TextStyle = fyne.TextStyle{Monospace: true}
	p.cwdLabel.TextSize = theme.TextSize() - 1

	return container.NewVBox(p.cwdLabel)
}

func (p *popupWindow) buildWorkspace() fyne.CanvasObject {
	p.inputHeading = brandSectionLabel(sectionAsk)

	prompt := canvas.NewText(inputPrompt, brandAccentGreen)
	prompt.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	prompt.TextSize = theme.TextSize() + 3

	hint := canvas.NewText(sendHintText, brandSecondaryText)
	hint.TextStyle = fyne.TextStyle{Monospace: true}
	hint.TextSize = theme.TextSize() - 3
	inputWithCursor := p.input.withFocusCursor()

	inputRow := container.NewBorder(
		nil, nil,
		vCenter(prompt),
		container.NewHBox(vCenter(hint), hStrut(12), vCenter(p.actionButton)),
		vCenter(inputWithCursor),
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

	askGroup := container.NewVBox(p.inputHeading, inputPanel)

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

func (p *popupWindow) buildMascotPanel() fyne.CanvasObject {
	p.robotIdle = robotMascot(brandAccentGreen)
	p.robotFrames = make([]fyne.Resource, mascotFrameCount)
	for i := range p.robotFrames {
		s := math.Sin(2 * math.Pi * float64(i) / float64(mascotFrameCount))
		// Legs scissor in opposition and the antenna sways with them: a walk.
		p.robotFrames[i] = robotMascotFrame(brandAccentGreen, mascotAntennaSwing*s, mascotLegSwing*s, -mascotLegSwing*s)
	}

	p.mascotImage = canvas.NewImageFromResource(p.robotIdle)
	p.mascotImage.FillMode = canvas.ImageFillContain
	p.mascotImage.SetMinSize(fyne.NewSize(mascotSize, mascotSize))
	mascot := container.NewGridWrap(fyne.NewSize(mascotSize, mascotSize), p.mascotImage)

	t1 := canvas.NewText(tagline1, brandMutedGreen)
	t1.TextStyle = fyne.TextStyle{Monospace: true}
	t2 := canvas.NewText(tagline2, brandMutedGreen)
	t2.TextStyle = fyne.TextStyle{Monospace: true}
	tagline := container.NewVBox(layout.NewSpacer(), t1, t2, layout.NewSpacer())

	// The wire between the agent and the host, with a packet that travels along
	// it during a request (see tickMascot); idle it is a faint dotted rule.
	p.dataLane = newDataLane(mascotDotCount)

	// Receiving end: the host/model the agent sends to (not the >_ mark, which
	// is the agent's own identity). It shakes and swells as the packet arrives.
	p.host = newHostNode(serverIcon(brandMutedGreen), serverIcon(brandAccentGreen))

	row := container.NewBorder(
		nil, nil,
		container.NewHBox(container.NewCenter(mascot), hStrut(16), tagline),
		container.NewHBox(hStrut(10), container.NewCenter(p.host)),
		p.dataLane,
	)
	return borderedBox(row, brandBorder)
}

// startMascotAnimation plays the walking wiggle and data-transfer dots while a
// request is in flight. It is idempotent.
func (p *popupWindow) startMascotAnimation() {
	if p.mascotImage == nil {
		return
	}
	if p.mascotAnim == nil {
		p.mascotAnim = fyne.NewAnimation(mascotAnimDuration, p.tickMascot)
		p.mascotAnim.Curve = fyne.AnimationLinear
		p.mascotAnim.RepeatCount = fyne.AnimationRepeatForever
	}
	p.mascotAnim.Start()
}

// stopMascotAnimation halts the animation and restores the resting mascot and
// the faint idle dotted rule.
func (p *popupWindow) stopMascotAnimation() {
	if p.mascotAnim != nil {
		p.mascotAnim.Stop()
	}
	if p.mascotImage != nil {
		p.mascotFrame = 0
		p.mascotImage.Resource = p.robotIdle
		p.mascotImage.Refresh()
	}
	if p.dataLane != nil {
		p.dataLane.SetIdle()
	}
	if p.host != nil {
		p.host.SetState(0, 0)
	}
}

// tickMascot advances the walking mascot frame and sends the packet along the
// data lane for a given animation progress in [0,1).
func (p *popupWindow) tickMascot(progress float32) {
	frame := int(progress*float32(mascotFrameCount)) % mascotFrameCount
	if frame != p.mascotFrame {
		p.mascotFrame = frame
		p.mascotImage.Resource = p.robotFrames[frame]
		p.mascotImage.Refresh()
	}

	// Triangle wave: the packet runs to the host and back, suggesting data sent
	// and received.
	tri := 1 - float32(math.Abs(float64(2*progress-1)))
	if p.dataLane != nil {
		p.dataLane.SetProgress(tri)
	}
	// The host reacts as the packet nears it, peaking on arrival, with a fast
	// jitter for the "shake".
	if p.host != nil {
		level := (tri - hostReceiveStart) / (1 - hostReceiveStart)
		shake := float32(math.Sin(float64(progress) * 2 * math.Pi * hostShakeFreq))
		p.host.SetState(level, shake)
	}
}

func (p *popupWindow) wireInteractions(app fyne.App) {
	win := p.window

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

func (p *popupWindow) setResponseHeading(text string) {
	if p.responseHeading.Text == text {
		return
	}
	p.responseHeading.Text = text
	p.responseHeading.Refresh()
}

func (p *popupWindow) setCwd(text string) {
	if p.cwdLabel.Text == text {
		return
	}
	p.cwdLabel.Text = text
	p.cwdLabel.Refresh()
}

func (p *popupWindow) setOutput(content string) {
	p.errorBody.Hide()
	p.outputBody.Show()
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

// unwrapMarkdownFence strips an outer ```markdown or ```md code fence so
// the inner content is parsed as rich text instead of a single code block.
// LLMs commonly wrap markdown responses this way.
func unwrapMarkdownFence(content string) string {
	trimmed := strings.TrimSpace(content)
	lower := strings.ToLower(trimmed)

	var prefixLen int
	switch {
	case strings.HasPrefix(lower, "```markdown\n"):
		prefixLen = len("```markdown\n")
	case strings.HasPrefix(lower, "```md\n"):
		prefixLen = len("```md\n")
	default:
		return content
	}

	inner := trimmed[prefixLen:]
	if !strings.HasSuffix(inner, "\n```") {
		return content
	}
	return inner[:len(inner)-4]
}

// decorateDollarMarkers prefixes prose paragraphs with the terminal "$" output
// marker from the brand response styling. It deliberately skips lines that
// carry markdown block syntax (headings, lists, quotes, tables, code fences,
// and indented code) so the marker never corrupts structured output; those
// blocks render normally. Continuation lines inside a paragraph are left as-is
// so they indent under the marked first line rather than repeating "$".
func decorateDollarMarkers(content string) string {
	lines := strings.Split(content, "\n")
	inFence := false
	atParagraphStart := true
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			atParagraphStart = false
			continue
		}
		if inFence {
			continue
		}
		if trimmed == "" {
			atParagraphStart = true
			continue
		}
		if atParagraphStart && isProseLine(line) {
			lines[i] = "$ " + line
		}
		atParagraphStart = false
	}
	return strings.Join(lines, "\n")
}

// colorizeDollarMarkers recolors the leading "$" output marker of each marked
// prose segment to the brand accent green, matching the terminal response
// styling in the mock while leaving the body text in the default color.
func colorizeDollarMarkers(segs []widget.RichTextSegment) []widget.RichTextSegment {
	out := make([]widget.RichTextSegment, 0, len(segs)+4)
	for _, seg := range segs {
		ts, ok := seg.(*widget.TextSegment)
		if !ok || !strings.HasPrefix(ts.Text, "$ ") {
			out = append(out, seg)
			continue
		}
		marker := &widget.TextSegment{Text: "$", Style: ts.Style}
		marker.Style.ColorName = theme.ColorNamePrimary
		rest := &widget.TextSegment{Text: ts.Text[1:], Style: ts.Style}
		out = append(out, marker, rest)
	}
	return out
}

// isProseLine reports whether a line is plain prose that should receive a "$"
// marker, as opposed to a markdown block element that must be left untouched.
func isProseLine(line string) bool {
	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
		return false // indented code / nested block
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	switch trimmed[0] {
	case '#', '-', '*', '+', '>', '|', '`', '=':
		return false
	}
	// Ordered list item: "1." / "2)" etc.
	if trimmed[0] >= '0' && trimmed[0] <= '9' {
		rest := strings.TrimLeft(trimmed, "0123456789")
		if strings.HasPrefix(rest, ".") || strings.HasPrefix(rest, ")") {
			return false
		}
	}
	return true
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

// showTestDialog presents the dev-only Test menu. Each entry runs its action
// and closes the dialog. The dialog is only reachable when the GUI is started
// in dev mode.
func (p *popupWindow) showTestDialog(tests []devTest) {
	win := p.window
	var dlg dialog.Dialog
	buttons := make([]fyne.CanvasObject, 0, len(tests))
	for _, t := range tests {
		run := t.run
		btn := widget.NewButton(t.name, func() {
			dlg.Hide()
			if run != nil {
				run()
			}
		})
		btn.Alignment = widget.ButtonAlignLeading
		buttons = append(buttons, btn)
	}
	content := container.NewVBox(buttons...)
	dlg = dialog.NewCustom("Test", "Close", content, win)
	dlg.Resize(fyne.NewSize(360, 0))
	dlg.Show()
}

func (p *popupWindow) showSettingsDialog(options settingsDialogOptions) {
	win := p.window
	currentEnvResult := options.EnvResult
	modelEntry := newSettingsTextEntry(options.InitialModel)
	errorLabel := widget.NewLabel("")
	errorLabel.Wrapping = fyne.TextWrapWord
	errorLabel.Importance = widget.DangerImportance

	// Hints are always present and single-line so changing their text never
	// changes the dialog height. A height change would resize and recenter the
	// dialog, moving the provider field without moving the already-open
	// autocomplete popup, leaving the popup covering the field's text.
	newHint := func() *widget.Label {
		l := widget.NewLabel("")
		l.Wrapping = fyne.TextWrapOff
		l.Truncation = fyne.TextTruncateEllipsis
		l.TextStyle = fyne.TextStyle{Italic: true}
		return l
	}
	modelHint := newHint()
	providerStatus := newProviderStatusIcon(win.Canvas())

	updateHints := func(provider string) {
		provider = strings.TrimSpace(provider)
		providerStatus.setStatus(providerReadinessStatusWithEnvironment(provider, currentEnvResult))
		if options.ModelForProvider != nil && slices.Contains(connector.SupportedProviders(), provider) {
			if model := strings.TrimSpace(options.ModelForProvider(provider)); model != "" {
				modelEntry.SetText(model)
			}
		}
		if def := connector.DefaultModelFor(provider); def != "" {
			modelHint.SetText("Default model: " + def)
		} else {
			modelHint.SetText("")
		}
	}

	providerInput := newProviderEntry(options.InitialProvider, updateHints)

	providerLabel := widget.NewLabelWithStyle("Provider", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	providerLabelBox := container.NewHBox(providerLabel, providerStatus)
	modelLabel := widget.NewLabelWithStyle("Model", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// A single FormLayout grid keeps the Provider and Model labels and inputs
	// aligned, and makes both inputs start at the same x with the same width.
	// Hint rows use an empty label cell so each hint aligns under its input.
	form := container.New(layout.NewFormLayout(),
		providerLabelBox, providerInput.withFocusCursor(),
		modelLabel, modelEntry.withFocusCursor(),
		widget.NewLabel(""), modelHint,
	)
	environmentSummary := environmentSummaryText(currentEnvResult)
	var dlg dialog.Dialog
	saveButton := widget.NewButton("Save", func() {
		provider := strings.TrimSpace(providerInput.Text)
		model := strings.TrimSpace(modelEntry.Text)
		if provider == "" {
			errorLabel.SetText("Provider cannot be empty.")
			return
		}
		if model == "" {
			errorLabel.SetText("Model cannot be empty.")
			return
		}
		if options.OnSave == nil {
			errorLabel.SetText("Settings cannot be saved.")
			return
		}
		if err := options.OnSave(provider, model); err != nil {
			errorLabel.SetText(err.Error())
			return
		}
		dlg.Hide()
	})
	saveButton.Importance = widget.HighImportance
	cancelButton := widget.NewButton("Cancel", func() {
		dlg.Hide()
	})
	versionLabel := widget.NewLabelWithStyle(options.Version, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	footer := container.NewHBox(versionLabel, layout.NewSpacer(), cancelButton, saveButton)
	contentObjects := []fyne.CanvasObject{form}
	if environmentSummary != "" {
		environmentLabel := widget.NewLabel(environmentSummary)
		environmentLabel.Wrapping = fyne.TextWrapWord
		contentObjects = append(contentObjects,
			widget.NewLabelWithStyle("Environment", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			environmentLabel,
		)
	}
	contentObjects = append(contentObjects, errorLabel, footer)
	content := container.NewVBox(contentObjects...)
	updateHints(options.InitialProvider)

	dlg = dialog.NewCustomWithoutButtons("Settings", content, win)
	if options.OnClosed != nil {
		dlg.SetOnClosed(options.OnClosed)
	}
	dlg.Resize(fyne.NewSize(520, 0))
	dlg.Show()
}

func environmentSummaryText(result EnvironmentLoadResult) string {
	lines := []string{}
	if result.EnvFileError != nil {
		lines = append(lines, "App env file: "+result.EnvFileError.Error())
	}
	if result.EnvFileWarning != nil {
		lines = append(lines, "App env file warning: "+result.EnvFileWarning.Error())
	}
	if result.ShellError != nil {
		lines = append(lines, "Shell import failed: "+result.ShellError.Error())
	}
	return strings.Join(lines, "\n")
}

func (p *popupWindow) resizeInput(value string) {
	rows := promptInputRows(value, p.input.Size().Width)
	p.input.SetMinRowsVisible(rows)
	p.input.Refresh()
	if content := p.window.Content(); content != nil {
		content.Refresh()
	}
	win := p.window
	current := win.Canvas().Size()
	height := current.Height
	if height < minWindowHeight {
		height = minWindowHeight
	}
	if height > maxWindowHeight {
		height = maxWindowHeight
	}
	win.Resize(fyne.NewSize(max(current.Width, defaultWindowWidth), height))
}

func promptInputRows(value string, width float32) int {
	rows := promptInputRowCount(value, width)
	if rows > maxInputRows {
		rows = maxInputRows
	}
	return rows
}

func promptInputRowCount(value string, width float32) int {
	charWidth := fyne.MeasureText("M", theme.TextSize(), fyne.TextStyle{Monospace: true}).Width
	cols := int((width - inputTextInset) / charWidth)
	if cols < 1 {
		cols = 1
	}

	rows := 0
	for _, line := range strings.Split(value, "\n") {
		lineRows := int(math.Ceil(float64(len([]rune(line))) / float64(cols)))
		if lineRows < 1 {
			lineRows = 1
		}
		rows += lineRows
	}
	if rows < 1 {
		rows = 1
	}
	return rows
}

func max(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
