package gui

import (
	"image/color"
	"slices"
	"strings"

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
	sidebarWidth        float32 = 176
	navIconSize         float32 = 16
	micRingSize         float32 = 54
	statusIndicatorSize float32 = 18

	providerStatusWidth  float32 = 28
	providerStatusHeight float32 = 24

	wordmarkText   = "TERMINAL AGENT"
	promptGlyph    = ">_"
	inputPrompt    = ">"
	sectionAsk     = "ASK THE TERMINAL AGENT"
	sectionResp    = "RESPONSE"
	sectionErr     = "ERROR"
	sendButtonText = "SEND  ›"
	stopButtonText = "STOP  ■"
	sendHintText   = "Ctrl + Enter to send"
	tagline1       = "Your terminal."
	tagline2       = "My context."
)

// asciiMascot is the line-based terminal agent that anchors the bottom of the
// workspace. It is intentionally simple and monochrome per the brand mascot
// guidance (a tiny process living inside the session, not a glossy avatar).
var asciiMascot = []string{
	"  ╭─────╮",
	" ╶┤ o o ├╴",
	"  │  ◡  │",
	"  ╰┬───┬╯",
	"   ╵   ╵",
}

var spinnerFrames = []string{"|", "/", "-", "\\"}

type popupWindow struct {
	window fyne.Window

	input        *popupEntry
	actionButton *widget.Button

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
	listenSubtitle *canvas.Text
	micRing        *canvas.Circle

	copyButton   *widget.Button
	exportButton *widget.Button

	clockLabel *canvas.Text
	cwdLabel   *canvas.Text

	testButton *widget.Button

	onSubmit      func()
	onEscape      func()
	onAction      func()
	onVoiceToggle func()
	onCopy        func()
	onExport      func()
	onSettings    func()
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
	voiceTriggerKey fyne.KeyName
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
	p.outputBody = container.NewPadded(outputField)

	errorLabel := widget.NewLabel("")
	errorLabel.Wrapping = fyne.TextWrapBreak
	errorLabel.Importance = widget.DangerImportance
	p.errorLabel = errorLabel
	p.errorBody = container.NewPadded(errorLabel)
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
	nav := container.NewVBox(
		newNavRow("CHAT", iconPathChat, true, nil),
		newNavRow("HISTORY", iconPathHistory, false, nil),
		newNavRow("ENV", iconPathEnv, false, nil),
		newNavRow("TOOLS", iconPathTools, false, nil),
		newNavRow("SETTINGS", iconPathSettings, false, func() {
			if p.onSettings != nil {
				p.onSettings()
			}
		}),
	)

	if devMode {
		p.testButton = widget.NewButton("Test", func() {
			if p.onTest != nil {
				p.onTest()
			}
		})
		p.testButton.Importance = widget.LowImportance
		nav.Add(p.testButton)
	}

	content := container.NewVBox(
		nav,
		layout.NewSpacer(),
		p.buildListenControl(),
		layout.NewSpacer(),
		p.buildConnectionStatus(),
	)

	bg := canvas.NewRectangle(brandPanel)
	panel := container.NewStack(bg, container.NewPadded(content))
	return container.New(&fixedWidthLayout{width: sidebarWidth}, panel)
}

func (p *popupWindow) buildListenControl() fyne.CanvasObject {
	ring := canvas.NewCircle(color.Transparent)
	ring.StrokeColor = brandAccentGreen
	ring.StrokeWidth = 1.5
	p.micRing = ring

	micImg := canvas.NewImageFromResource(lineIcon("mic", iconPathMic, brandAccentGreen))
	micImg.FillMode = canvas.ImageFillContain
	micImg.SetMinSize(fyne.NewSize(22, 22))

	ringStack := container.NewGridWrap(
		fyne.NewSize(micRingSize, micRingSize),
		container.NewStack(ring, container.NewCenter(micImg)),
	)

	p.listenButton = widget.NewButton("LISTEN", nil)
	p.listenButton.Importance = widget.LowImportance

	p.listenSubtitle = canvas.NewText("Press to toggle mic", brandSecondaryText)
	p.listenSubtitle.Alignment = fyne.TextAlignCenter
	p.listenSubtitle.TextSize = theme.TextSize() - 2

	return container.NewVBox(
		container.NewCenter(ringStack),
		container.NewCenter(p.listenButton),
		container.NewCenter(p.listenSubtitle),
	)
}

func (p *popupWindow) buildConnectionStatus() fyne.CanvasObject {
	connected := canvas.NewText("CONNECTED", brandMutedGreen)
	connected.TextStyle = fyne.TextStyle{Monospace: true}
	connected.TextSize = theme.TextSize() - 1

	host := canvas.NewText("localhost", brandSecondaryText)
	host.TextSize = theme.TextSize() - 2

	p.cwdLabel = canvas.NewText("~/", brandSecondaryText)
	p.cwdLabel.TextSize = theme.TextSize() - 2

	p.clockLabel = canvas.NewText("", brandSecondaryText)
	p.clockLabel.TextSize = theme.TextSize() - 2

	return container.NewVBox(
		container.NewHBox(newStatusDot(brandAccentGreen), connected),
		host,
		p.cwdLabel,
		p.clockLabel,
	)
}

func (p *popupWindow) buildWorkspace() fyne.CanvasObject {
	askHeading := brandSectionLabel(sectionAsk)

	prompt := canvas.NewText(inputPrompt, brandAccentGreen)
	prompt.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	prompt.TextSize = theme.TextSize() + 3

	hint := canvas.NewText(sendHintText, brandSecondaryText)
	hint.TextSize = theme.TextSize() - 2

	inputRow := container.NewBorder(
		nil, nil,
		container.NewVBox(layout.NewSpacer(), prompt, layout.NewSpacer()),
		container.NewHBox(container.NewVBox(layout.NewSpacer(), hint, layout.NewSpacer()), p.actionButton),
		container.NewVBox(layout.NewSpacer(), p.input, layout.NewSpacer()),
	)
	inputPanel := borderedBox(container.NewPadded(inputRow), brandBorderBright)

	p.responseHeading = brandSectionLabel(sectionResp)

	metaRow := container.NewVBox(brandSeparator(), container.NewPadded(p.metaLabel))
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

	askGroup := container.NewVBox(askHeading, inputPanel)

	return container.NewBorder(
		askGroup,
		p.buildMascotPanel(),
		nil, nil,
		p.responseSection,
	)
}

func (p *popupWindow) buildMascotPanel() fyne.CanvasObject {
	mascotLines := container.NewVBox()
	for _, line := range asciiMascot {
		t := canvas.NewText(line, brandAccentGreen)
		t.TextStyle = fyne.TextStyle{Monospace: true}
		t.TextSize = theme.TextSize() - 1
		mascotLines.Add(t)
	}

	t1 := canvas.NewText(tagline1, brandMutedGreen)
	t1.TextStyle = fyne.TextStyle{Monospace: true}
	t2 := canvas.NewText(tagline2, brandMutedGreen)
	t2.TextStyle = fyne.TextStyle{Monospace: true}
	tagline := container.NewVBox(layout.NewSpacer(), t1, t2, layout.NewSpacer())

	dots := canvas.NewText(strings.Repeat(". ", 28), brandBorderBright)
	dots.TextSize = theme.TextSize() - 1
	tail := canvas.NewText(promptGlyph, brandAccentGreen)
	tail.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	dottedTail := container.NewHBox(layout.NewSpacer(), container.NewCenter(dots), container.NewCenter(tail))

	row := container.NewBorder(
		nil, nil,
		container.NewHBox(mascotLines, widget.NewLabel("  "), tagline),
		nil,
		dottedTail,
	)
	return borderedBox(container.NewPadded(row), brandBorder)
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
		p.listenButton.SetText("LISTENING")
		p.listenSubtitle.Text = "Press to stop"
		p.micRing.StrokeColor = brandAccentGreen
	case voice.StateTranscribing:
		p.listenButton.SetText("WORKING")
		p.listenSubtitle.Text = "Transcribing…"
		p.micRing.StrokeColor = brandMutedGreen
	default:
		if enabled {
			p.listenButton.SetText("LISTEN")
			p.listenSubtitle.Text = "Press to toggle mic"
			p.micRing.StrokeColor = brandAccentGreen
		} else {
			p.listenButton.SetText("MIC OFF")
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

func (p *popupWindow) setMeta(text string) {
	if p.metaLabel.Text == text {
		return
	}
	p.metaLabel.Text = text
	p.metaLabel.Refresh()
}

func (p *popupWindow) setResponseHeading(text string) {
	if p.responseHeading.Text == text {
		return
	}
	p.responseHeading.Text = text
	p.responseHeading.Refresh()
}

func (p *popupWindow) setClock(text string) {
	if p.clockLabel.Text == text {
		return
	}
	p.clockLabel.Text = text
	p.clockLabel.Refresh()
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
	segs := renderMarkdown(decorateDollarMarkers(unwrapMarkdownFence(content)))
	p.outputField.Segments = colorizeDollarMarkers(segs)
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
	modelEntry := widget.NewEntry()
	modelEntry.SetText(options.InitialModel)
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
		providerLabelBox, providerInput,
		modelLabel, modelEntry,
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
	rows := strings.Count(value, "\n") + 1
	if rows < 1 {
		rows = 1
	}
	if rows > maxInputRows {
		rows = maxInputRows
	}
	p.input.SetMinRowsVisible(rows)
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

func max(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
