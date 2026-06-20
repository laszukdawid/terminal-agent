package gui

import (
	"math"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	maxInputRows                    = 3
	promptNativeCursorWidth         = 2
	inputTextInset          float32 = 24
)

type popupEntry struct {
	widget.Entry
	app             fyne.App
	onEscape        func()
	onSubmit        func()
	onVoiceToggle   func()
	onFocusChanged  func(bool)
	voiceTriggerKey fyne.KeyName
}

type settingsTextEntry struct {
	widget.Entry
	onEscape func()
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

// TypedKey routes Escape to onEscape when set so the Settings dialog can close
// on Escape while the model field has focus; every other key keeps the default
// entry editing behavior.
func (e *settingsTextEntry) TypedKey(key *fyne.KeyEvent) {
	if key.Name == fyne.KeyEscape && e.onEscape != nil {
		e.onEscape()
		return
	}
	e.Entry.TypedKey(key)
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

func (p *popupWindow) resizeInput(value string) {
	rows := promptInputRows(value, p.input.Size().Width)
	p.input.SetMinRowsVisible(rows)
	p.input.Refresh()
	if content := p.window.Content(); content != nil {
		content.Refresh()
	}
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
