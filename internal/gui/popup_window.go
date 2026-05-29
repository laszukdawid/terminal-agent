package gui

import (
	"image/color"
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
	defaultWindowWidth    float32 = 720
	defaultWindowHeight   float32 = 280
	minWindowHeight       float32 = 240
	maxWindowHeight       float32 = 760
	maxInputRows                  = 3
	compactOutputHeight   float32 = 110
	statusMinWidth        float32 = 130
	statusIndicatorSize   float32 = 18
	maxVisibleOutputLines         = 5
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
	outputField     *readOnlyEntry
	headerStatus    *widget.Label
	headerBrain     *canvas.Text
	headerSpinner   *canvas.Text
	headerStatusBox fyne.CanvasObject
	modelLabel      *widget.Label
	actionButton    *widget.Button
	copyButton      *widget.Button
	settingsButton  *widget.Button
	answerPanel     *fyne.Container

	onSubmit     func()
	onShiftEnter func()
	onEscape     func()
	onAction     func()
	onCopy       func()
	onSettings   func()
	onInput      func(string)
}

type popupEntry struct {
	widget.Entry
	app      fyne.App
	onEscape func()
	onSubmit func()
}

type readOnlyEntry struct {
	widget.Entry
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

func newReadOnlyEntry() *readOnlyEntry {
	entry := &readOnlyEntry{}
	entry.ExtendBaseWidget(entry)
	entry.MultiLine = true
	entry.Wrapping = fyne.TextWrapWord
	entry.Scroll = fyne.ScrollVerticalOnly
	return entry
}

func (e *readOnlyEntry) TypedRune(rune) {}

func (e *readOnlyEntry) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyUp, fyne.KeyDown, fyne.KeyLeft, fyne.KeyRight, fyne.KeyPageUp, fyne.KeyPageDown, fyne.KeyHome, fyne.KeyEnd:
		e.Entry.TypedKey(key)
	}
}

func newPopupWindow(app fyne.App) *popupWindow {
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

	outputField := newReadOnlyEntry()
	outputField.SetMinRowsVisible(6)

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

	questionCard := withBackground(questionLabel, color.NRGBA{R: 232, G: 235, B: 239, A: 255})
	questionCard.Hide()
	inputCard := withBackground(input, theme.Color(theme.ColorNameInputBackground))
	outputCard := withBackground(outputField, theme.Color(theme.ColorNameInputBackground))
	outputCard.Resize(fyne.NewSize(0, compactOutputHeight))

	answerHeader := container.NewHBox(
		answerHeading,
		layout.NewSpacer(),
		copyButton,
	)
	answerMeta := container.NewVBox(requestHeading, questionCard, answerHeader)
	answerPanel := container.NewBorder(answerMeta, nil, nil, nil, outputCard)
	toolbar := container.NewHBox(actionButton, layout.NewSpacer(), settingsButton)
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
		headerStatus:    headerStatus,
		headerBrain:     headerBrain,
		headerSpinner:   headerSpinner,
		headerStatusBox: headerStatusBox,
		modelLabel:      modelLabel,
		actionButton:    actionButton,
		copyButton:      copyButton,
		settingsButton:  settingsButton,
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

	window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyL, Modifier: fyne.KeyModifierShortcutDefault}, func(shortcut fyne.Shortcut) {
		p.window.Canvas().Focus(p.input)
	})

	return p
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

func (p *popupWindow) showSettingsDialog(initialProvider, initialModel string, onSave func(provider, model string) error) {
	modelEntry := widget.NewEntry()
	modelEntry.SetText(initialModel)
	errorLabel := widget.NewLabel("")
	errorLabel.Wrapping = fyne.TextWrapWord
	errorLabel.Importance = widget.DangerImportance

	setupHint := widget.NewLabel("")
	setupHint.Wrapping = fyne.TextWrapWord
	setupHint.Importance = widget.LowImportance
	modelHint := widget.NewLabel("")
	modelHint.Wrapping = fyne.TextWrapWord
	modelHint.Importance = widget.LowImportance

	updateHints := func(provider string) {
		provider = strings.TrimSpace(provider)
		if hint := providerSetupHint(provider); hint != "" {
			setupHint.SetText(hint)
			setupHint.Show()
		} else {
			setupHint.Hide()
		}
		if def := connector.DefaultModelFor(provider); def != "" {
			modelHint.SetText("Default model: " + def)
			modelHint.Show()
		} else {
			modelHint.Hide()
		}
	}

	providerInput := newProviderEntry(initialProvider, updateHints)

	content := container.NewVBox(
		widget.NewForm(widget.NewFormItem("Provider", providerInput)),
		setupHint,
		widget.NewForm(widget.NewFormItem("Model", modelEntry)),
		modelHint,
		errorLabel,
	)
	updateHints(initialProvider)

	var dlg dialog.Dialog
	dlg = dialog.NewCustomConfirm("Settings", "Save", "Cancel", content, func(confirm bool) {
		if !confirm {
			return
		}

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
		if err := onSave(provider, model); err != nil {
			errorLabel.SetText(err.Error())
			return
		}
		dlg.Hide()
	}, p.window)
	dlg.Resize(fyne.NewSize(420, 0))
	dlg.Show()
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
