package gui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	defaultWindowWidth  float32 = 760
	defaultWindowHeight float32 = 420
	minWindowHeight     float32 = 360
	maxWindowHeight     float32 = 820
	maxInputRows                = 3
)

type popupWindow struct {
	window        fyne.Window
	input         *widget.Entry
	questionLabel *widget.Label
	outputLabel   *widget.Label
	statusLabel   *widget.Label
	modelLabel    *widget.Label
	submitButton  *widget.Button
	cancelButton  *widget.Button
	copyButton    *widget.Button
	outputScroll  *container.Scroll
	answerPanel   *fyne.Container

	onSubmit     func()
	onShiftEnter func()
	onEscape     func()
	onCancel     func()
	onCopy       func()
	onInput      func(string)
}

func newPopupWindow(app fyne.App) *popupWindow {
	window := app.NewWindow("Terminal Agent")
	window.Resize(fyne.NewSize(defaultWindowWidth, defaultWindowHeight))
	window.SetFixedSize(false)
	window.CenterOnScreen()

	input := widget.NewMultiLineEntry()
	input.SetPlaceHolder("Ask Terminal Agent")
	input.Wrapping = fyne.TextWrapWord
	input.Scroll = fyne.ScrollVerticalOnly
	input.SetMinRowsVisible(1)

	questionLabel := widget.NewLabel("")
	questionLabel.Wrapping = fyne.TextWrapWord
	questionLabel.Selectable = true

	outputLabel := widget.NewLabel("")
	outputLabel.Wrapping = fyne.TextWrapWord
	outputLabel.Selectable = true

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord

	modelLabel := widget.NewLabel("")
	modelLabel.TextStyle = fyne.TextStyle{Italic: true}
	modelLabel.Alignment = fyne.TextAlignTrailing
	modelLabel.Importance = widget.LowImportance

	submitButton := widget.NewButton("Submit", nil)
	cancelButton := widget.NewButton("Cancel", nil)
	copyButton := widget.NewButton("Copy", nil)
	cancelButton.Disable()
	copyButton.Disable()

	questionCard := withBackground(questionLabel, color.NRGBA{R: 34, G: 39, B: 46, A: 255})
	questionCard.Hide()
	outputScroll := container.NewVScroll(outputLabel)
	outputScroll.SetMinSize(fyne.NewSize(0, 160))

	answerPanel := container.NewVBox(questionCard, outputScroll)

	content := container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Terminal Agent", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			input,
			statusLabel,
		),
		container.NewVBox(
			container.NewHBox(submitButton, cancelButton, copyButton, layout.NewSpacer(), modelLabel),
		),
		nil,
		nil,
		answerPanel,
	)

	window.SetContent(container.NewPadded(content))

	p := &popupWindow{
		window:        window,
		input:         input,
		questionLabel: questionLabel,
		outputLabel:   outputLabel,
		statusLabel:   statusLabel,
		modelLabel:    modelLabel,
		submitButton:  submitButton,
		cancelButton:  cancelButton,
		copyButton:    copyButton,
		outputScroll:  outputScroll,
		answerPanel:   answerPanel,
	}

	input.OnChanged = func(value string) {
		p.resizeInput(value)
		if p.onInput != nil {
			p.onInput(value)
		}
	}
	input.OnSubmitted = func(_ string) {
		if p.onSubmit != nil {
			p.onSubmit()
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

	window.Canvas().SetOnTypedKey(func(key *fyne.KeyEvent) {
		switch key.Name {
		case fyne.KeyEscape:
			if p.onEscape != nil {
				p.onEscape()
			}
		}
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
