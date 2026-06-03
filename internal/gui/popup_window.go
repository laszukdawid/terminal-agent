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
)

const (
	defaultWindowWidth   float32 = 720
	defaultWindowHeight  float32 = 280
	minWindowHeight      float32 = 240
	maxWindowHeight      float32 = 760
	maxInputRows                 = 3
	compactOutputHeight  float32 = 110
	statusMinWidth       float32 = 130
	statusIndicatorSize  float32 = 18
	providerStatusWidth  float32 = 28
	providerStatusHeight float32 = 24
)

var spinnerFrames = []string{"|", "/", "-", "\\"}

type popupWindow struct {
	window          fyne.Window
	input           *popupEntry
	inputCard       fyne.CanvasObject
	requestHeading  *widget.Label
	answerHeading   *widget.Label
	answerHeader    *fyne.Container
	answerMeta      *fyne.Container
	questionCard    fyne.CanvasObject
	questionLabel   *canvas.Text
	outputField     *widget.RichText
	outputBody      fyne.CanvasObject
	errorLabel      *widget.Label
	errorBody       fyne.CanvasObject
	outputScroll    *container.Scroll
	lastRendered    string
	lastError       string
	headerStatus    *widget.Label
	headerBrain     *canvas.Text
	headerSpinner   *canvas.Text
	headerStatusBox fyne.CanvasObject
	modelLabel      *widget.Label
	actionButton    *widget.Button
	copyButton      *widget.Button
	settingsButton  *widget.Button
	testButton      *widget.Button
	answerPanel     *fyne.Container

	onSubmit     func()
	onShiftEnter func()
	onEscape     func()
	onAction     func()
	onCopy       func()
	onSettings   func()
	onTest       func()
	onInput      func(string)
	onQuit       func()
}

type popupEntry struct {
	widget.Entry
	app      fyne.App
	onEscape func()
	onSubmit func()
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
	window := app.NewWindow("Terminal Agent")
	window.Resize(fyne.NewSize(defaultWindowWidth, defaultWindowHeight))
	window.SetFixedSize(false)
	window.CenterOnScreen()

	input := newPopupEntry(app)
	input.SetPlaceHolder("Ask Terminal Agent")
	input.SetMinRowsVisible(1)

	requestHeading := widget.NewLabelWithStyle("Request", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	answerHeading := widget.NewLabelWithStyle("Response", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	questionLabel := canvas.NewText("", color.Black)
	questionLabel.Alignment = fyne.TextAlignLeading
	questionLabel.TextSize = theme.TextSize()

	outputField := widget.NewRichText()
	outputField.Wrapping = fyne.TextWrapWord
	outputBody := withBackground(outputField, color.NRGBA{R: 248, G: 249, B: 251, A: 255})
	errorLabel := widget.NewLabel("")
	errorLabel.Wrapping = fyne.TextWrapBreak
	errorLabel.Importance = widget.DangerImportance
	errorBody := withBackground(errorLabel, color.NRGBA{R: 255, G: 245, B: 245, A: 255})
	errorBody.Hide()
	outputStack := container.NewStack(outputBody, errorBody)
	outputScroll := container.NewVScroll(outputStack)
	outputScroll.SetMinSize(fyne.NewSize(0, compactOutputHeight))

	headerStatus := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	headerStatus.Alignment = fyne.TextAlignCenter
	headerStatus.Wrapping = fyne.TextWrapOff
	headerStatus.Importance = widget.MediumImportance
	headerBrain := canvas.NewText("🧠", theme.PrimaryColor())
	headerBrain.Alignment = fyne.TextAlignCenter
	headerBrain.TextSize = theme.TextSize() + 2
	headerBrain.Hide()
	headerSpinner := canvas.NewText(spinnerFrames[0], theme.PrimaryColor())
	headerSpinner.Alignment = fyne.TextAlignCenter
	headerSpinner.TextSize = theme.TextSize() + 4
	headerSpinner.Hide()

	modelLabel := widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})

	actionButton := widget.NewButton("Submit", nil)
	copyButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), nil)
	settingsButton := widget.NewButton("Settings", nil)
	copyButton.Disable()

	var testButton *widget.Button
	if devMode {
		testButton = widget.NewButton("Test", nil)
	}

	questionCard := withBackground(questionLabel, color.NRGBA{R: 232, G: 235, B: 239, A: 255})
	questionCard.Hide()
	inputCard := withBackground(input, theme.Color(theme.ColorNameInputBackground))
	outputCard := outputScroll

	answerHeader := container.NewHBox(
		answerHeading,
		layout.NewSpacer(),
		copyButton,
	)
	answerMeta := container.NewVBox(requestHeading, questionCard, answerHeader)
	answerPanel := container.NewBorder(answerMeta, nil, nil, nil, outputCard)
	toolbar := container.NewHBox(actionButton, layout.NewSpacer())
	if testButton != nil {
		toolbar.Add(testButton)
	}
	toolbar.Add(settingsButton)
	statusSlot := container.NewGridWrap(
		fyne.NewSize(statusMinWidth, max(headerStatus.MinSize().Height, statusIndicatorSize)),
		container.NewCenter(headerStatus),
	)
	headerStatusBox := container.NewMax(statusSlot, container.NewCenter(headerBrain), container.NewCenter(headerSpinner))

	content := container.NewBorder(
		container.New(
			layout.NewVBoxLayout(),
			container.NewHBox(
				widget.NewLabelWithStyle("Terminal Agent", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				layout.NewSpacer(),
				headerStatusBox,
				layout.NewSpacer(),
				modelLabel,
			),
			inputCard,
		),
		toolbar,
		nil,
		nil,
		answerPanel,
	)

	window.SetContent(container.NewPadded(content))

	p := &popupWindow{
		window:          window,
		input:           input,
		inputCard:       inputCard,
		questionCard:    questionCard,
		requestHeading:  requestHeading,
		answerHeading:   answerHeading,
		answerHeader:    answerHeader,
		answerMeta:      answerMeta,
		questionLabel:   questionLabel,
		outputField:     outputField,
		outputBody:      outputBody,
		errorLabel:      errorLabel,
		errorBody:       errorBody,
		outputScroll:    outputScroll,
		headerStatus:    headerStatus,
		headerBrain:     headerBrain,
		headerSpinner:   headerSpinner,
		headerStatusBox: headerStatusBox,
		modelLabel:      modelLabel,
		actionButton:    actionButton,
		copyButton:      copyButton,
		settingsButton:  settingsButton,
		testButton:      testButton,
		answerPanel:     answerPanel,
	}
	input.onEscape = func() {
		if p.onEscape != nil {
			p.onEscape()
		}
	}
	input.onSubmit = func() {
		if p.onSubmit != nil {
			p.onSubmit()
		}
	}
	input.OnChanged = func(value string) {
		p.resizeInput(value)
		if p.onInput != nil {
			p.onInput(value)
		}
	}
	actionButton.OnTapped = func() {
		if p.onAction != nil {
			p.onAction()
		}
	}
	copyButton.OnTapped = func() {
		if p.onCopy != nil {
			p.onCopy()
		}
	}
	settingsButton.OnTapped = func() {
		if p.onSettings != nil {
			p.onSettings()
		}
	}
	if testButton != nil {
		testButton.OnTapped = func() {
			if p.onTest != nil {
				p.onTest()
			}
		}
	}

	window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyL, Modifier: fyne.KeyModifierShortcutDefault}, func(shortcut fyne.Shortcut) {
		p.window.Canvas().Focus(p.input)
	})
	window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyQ, Modifier: fyne.KeyModifierShortcutDefault}, func(shortcut fyne.Shortcut) {
		if p.onQuit != nil {
			p.onQuit()
		}
	})

	return p
}

func (p *popupWindow) setOutput(content string) {
	p.errorBody.Hide()
	p.outputBody.Show()
	if p.lastRendered == content {
		return
	}
	p.lastRendered = content
	p.outputField.Segments = renderMarkdown(unwrapMarkdownFence(content))
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

func (p *popupWindow) setStatus(status string, isRunning bool, spinnerFrame int) {
	p.headerBrain.Color = theme.PrimaryColor()
	p.headerBrain.Refresh()
	p.headerSpinner.Color = theme.PrimaryColor()
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
	dlg = dialog.NewCustom("Test", "Close", content, p.window)
	dlg.Resize(fyne.NewSize(360, 0))
	dlg.Show()
}

func (p *popupWindow) showSettingsDialog(options settingsDialogOptions) {
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
	providerStatus := newProviderStatusIcon(p.window.Canvas())

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

	dlg = dialog.NewCustomWithoutButtons("Settings", content, p.window)
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
	current := p.window.Canvas().Size()
	height := current.Height
	if height < minWindowHeight {
		height = minWindowHeight
	}
	if height > maxWindowHeight {
		height = maxWindowHeight
	}
	p.window.Resize(fyne.NewSize(max(current.Width, defaultWindowWidth), height))
}

func withBackground(content fyne.CanvasObject, fill color.Color) fyne.CanvasObject {
	rect := canvas.NewRectangle(fill)
	rect.CornerRadius = theme.Size(theme.SizeNameInputRadius)
	return container.NewPadded(container.NewStack(rect, container.NewPadded(content)))
}

func max(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
