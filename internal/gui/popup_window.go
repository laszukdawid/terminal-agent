package gui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	defaultWindowWidth  float32 = 720
	defaultWindowHeight float32 = 280
	minWindowHeight     float32 = 240
	maxWindowHeight     float32 = 760
	maxInputRows                = 3
	compactOutputHeight float32 = 110
	maxVisibleOutputLines       = 5
)

type popupWindow struct {
	window        fyne.Window
	input         *popupEntry
	inputCard     fyne.CanvasObject
	requestHeading *widget.Label
	answerHeading *widget.Label
	questionLabel *widget.Label
	outputLabel   *widget.Label
	statusLabel   *widget.Label
	modelLabel    *canvas.Text
	outputScroll  *container.Scroll
	submitButton  *widget.Button
	cancelButton  *widget.Button
	copyButton    *widget.Button
	answerPanel   *fyne.Container

	onSubmit     func()
	onShiftEnter func()
	onEscape     func()
	onCancel     func()
	onCopy       func()
	onInput      func(string)
}

type popupEntry struct {
	widget.Entry
	app      fyne.App
	onEscape func()
	onSubmit func()
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
	questionLabel := widget.NewLabel("")
	questionLabel.Wrapping = fyne.TextWrapWord
	questionLabel.Selectable = true

	outputLabel := widget.NewLabel("")
	outputLabel.Wrapping = fyne.TextWrapWord
	outputLabel.Selectable = true

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord
	statusLabel.Importance = widget.LowImportance

	modelLabel := canvas.NewText("", color.NRGBA{R: 232, G: 235, B: 239, A: 255})
	modelLabel.Alignment = fyne.TextAlignTrailing
	modelLabel.TextStyle = fyne.TextStyle{Bold: true}

	submitButton := widget.NewButton("Submit", nil)
	cancelButton := widget.NewButton("Cancel", nil)
	copyButton := widget.NewButton("Copy", nil)
	cancelButton.Disable()
	copyButton.Disable()

	questionCard := withBackground(questionLabel, color.NRGBA{R: 34, G: 39, B: 46, A: 255})
	questionCard.Hide()
	inputCard := withBackground(input, color.NRGBA{R: 28, G: 32, B: 38, A: 255})
	outputScroll := container.NewVScroll(outputLabel)
	outputScroll.SetMinSize(fyne.NewSize(0, compactOutputHeight))

	answerPanel := container.NewBorder(
		container.NewVBox(requestHeading, questionCard, answerHeading),
		nil,
		nil,
		nil,
		outputScroll,
	)
	toolbar := container.NewHBox(submitButton, cancelButton, copyButton, layout.NewSpacer())

	content := container.NewBorder(
		container.NewVBox(
			container.NewHBox(
				widget.NewLabelWithStyle("Terminal Agent", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
				layout.NewSpacer(),
				modelLabel,
			),
			inputCard,
			statusLabel,
		),
		toolbar,
		nil,
		nil,
		answerPanel,
	)

	window.SetContent(container.NewPadded(content))

	p := &popupWindow{
		window:        window,
		input:         input,
		inputCard:     inputCard,
		requestHeading: requestHeading,
		answerHeading: answerHeading,
		questionLabel: questionLabel,
		outputLabel:   outputLabel,
		statusLabel:   statusLabel,
		modelLabel:    modelLabel,
		outputScroll:  outputScroll,
		submitButton:  submitButton,
		cancelButton:  cancelButton,
		copyButton:    copyButton,
		answerPanel:   answerPanel,
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
	submitButton.OnTapped = func() {
		if p.onSubmit != nil {
			p.onSubmit()
		}
	}
	cancelButton.OnTapped = func() {
		if p.onCancel != nil {
			p.onCancel()
		}
	}
	copyButton.OnTapped = func() {
		if p.onCopy != nil {
			p.onCopy()
		}
	}

	window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyL, Modifier: fyne.KeyModifierShortcutDefault}, func(shortcut fyne.Shortcut) {
		p.window.Canvas().Focus(p.input)
	})

	return p
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
	p.outputScroll.SetMinSize(fyne.NewSize(0, p.outputHeight()))
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

func (p *popupWindow) outputHeight() float32 {
	lineCount := strings.Count(p.outputLabel.Text, "\n") + 1
	if lineCount < 1 {
		lineCount = 1
	}
	if lineCount > maxVisibleOutputLines {
		lineCount = maxVisibleOutputLines
	}
	lineHeight := theme.TextSize() + theme.Padding()
	height := float32(lineCount)*lineHeight + theme.Padding()*2
	if height < compactOutputHeight {
		return compactOutputHeight
	}
	return height
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
