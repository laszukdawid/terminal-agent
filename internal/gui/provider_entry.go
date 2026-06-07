package gui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xwidget "fyne.io/x/fyne/widget"

	"github.com/laszukdawid/terminal-agent/internal/connector"
)

type providerEntry struct {
	xwidget.CompletionEntry

	shown []string
	query string

	onFocusChanged func(bool)
	focusCursor    *canvas.Rectangle
	cursorAnim     *fyne.Animation
}

// newProviderEntry builds an editable autocomplete field for choosing a
// provider, pre-filled with initial. The user may type any value; suggestions
// are filtered from the connector registry by case-insensitive substring, and
// the matching substring is emphasized in each suggestion.
func newProviderEntry(initial string, onChange func(string)) *providerEntry {
	e := &providerEntry{}
	e.ExtendBaseWidget(e)
	e.Options = connector.SupportedProviders()
	e.Wrapping = fyne.TextWrap(fyne.TextTruncateClip)

	display := func(opts []string, q string) {
		e.shown = opts
		e.query = q
		e.SetOptions(opts)
		if len(opts) == 0 {
			e.HideCompletion()
			return
		}
		e.ShowCompletion()
	}

	e.CustomCreate = func() fyne.CanvasObject {
		rt := widget.NewRichText(&widget.TextSegment{Style: widget.RichTextStyleInline, Text: " "})
		rt.Wrapping = fyne.TextWrapOff
		return rt
	}
	e.CustomUpdate = func(id widget.ListItemID, o fyne.CanvasObject) {
		rt, ok := o.(*widget.RichText)
		if !ok {
			return
		}
		text := ""
		if id >= 0 && id < len(e.shown) {
			text = e.shown[id]
		}
		rt.Segments = matchSegments(text, e.query)
		rt.Refresh()
	}

	e.SetText(initial)
	e.OnChanged = func(s string) {
		display(filterProviders(connector.SupportedProviders(), s), s)
		if onChange != nil {
			onChange(s)
		}
	}
	return e
}

func (e *providerEntry) FocusGained() {
	e.CompletionEntry.FocusGained()
	if e.onFocusChanged != nil {
		e.onFocusChanged(true)
	}
	if e.Text == "" {
		e.shown = connector.SupportedProviders()
		e.query = ""
		e.SetOptions(e.shown)
		e.ShowCompletion()
	}
}

func (e *providerEntry) FocusLost() {
	e.CompletionEntry.FocusLost()
	if e.onFocusChanged != nil {
		e.onFocusChanged(false)
	}
}

func (e *providerEntry) withFocusCursor() fyne.CanvasObject {
	cursor := canvas.NewRectangle(brandAccentGreen)
	cursor.Hide()
	e.focusCursor = cursor

	wrapped := container.New(&providerCursorLayout{entry: e, cursor: cursor}, e, cursor)
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

func (e *providerEntry) positionFocusCursor() {
	if e.focusCursor == nil {
		return
	}
	textSize := fyne.MeasureText("M", theme.TextSize(), e.TextStyle)
	e.focusCursor.Resize(fyne.NewSize(promptCursorWidth, textSize.Height))
	e.focusCursor.Move(e.CursorPosition())
	e.focusCursor.Refresh()
}

func (e *providerEntry) startFocusCursor() {
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

func (e *providerEntry) stopFocusCursor() {
	if e.cursorAnim != nil {
		e.cursorAnim.Stop()
		e.cursorAnim = nil
	}
	if e.focusCursor != nil {
		e.focusCursor.Hide()
	}
}

type providerCursorLayout struct {
	entry  *providerEntry
	cursor *canvas.Rectangle
}

func (l *providerCursorLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return l.entry.MinSize()
}

func (l *providerCursorLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	l.entry.Resize(size)
	l.entry.Move(fyne.NewPos(0, 0))
	l.entry.positionFocusCursor()
}

// matchSegments splits text into RichText segments, emphasizing the first
// case-insensitive occurrence of query in bold and the theme's accent color.
// With an empty query (or no match) the whole text is returned as a single
// plain inline segment.
func matchSegments(text, query string) []widget.RichTextSegment {
	plain := widget.RichTextStyleInline

	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []widget.RichTextSegment{&widget.TextSegment{Style: plain, Text: text}}
	}
	idx := strings.Index(strings.ToLower(text), q)
	if idx < 0 {
		return []widget.RichTextSegment{&widget.TextSegment{Style: plain, Text: text}}
	}

	matched := plain
	matched.TextStyle = fyne.TextStyle{Bold: true}
	matched.ColorName = theme.ColorNamePrimary

	end := idx + len(q)
	segs := make([]widget.RichTextSegment, 0, 3)
	if idx > 0 {
		segs = append(segs, &widget.TextSegment{Style: plain, Text: text[:idx]})
	}
	segs = append(segs, &widget.TextSegment{Style: matched, Text: text[idx:end]})
	if end < len(text) {
		segs = append(segs, &widget.TextSegment{Style: plain, Text: text[end:]})
	}
	return segs
}
