package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	// infoIconGlyph is the circled-i drawn for an info icon.
	infoIconGlyph = "ⓘ"
	// infoIconWidth/Height bound the tappable area around the glyph so the icon
	// stays small next to a form label yet remains comfortable to click.
	infoIconWidth  float32 = 18
	infoIconHeight float32 = 18
	// infoPopupWidth caps the explanation popup so a long message wraps onto a
	// few short lines rather than stretching across the window.
	infoPopupWidth float32 = 320
)

// infoIcon is a small, tappable "ⓘ" glyph that reveals a short explanation in a
// popup when clicked. The popup is a non-modal overlay, so a click anywhere
// outside it (including on the icon again) dismisses it.
type infoIcon struct {
	widget.BaseWidget
	canvas  fyne.Canvas
	glyph   *canvas.Text
	message string
}

func newInfoIcon(c fyne.Canvas, message string) *infoIcon {
	i := &infoIcon{
		canvas:  c,
		message: message,
		glyph:   canvas.NewText(infoIconGlyph, currentBrandPalette().accentGreen),
	}
	i.glyph.TextSize = theme.TextSize()
	i.glyph.TextStyle = fyne.TextStyle{Bold: true}
	i.ExtendBaseWidget(i)
	return i
}

// Tapped shows the explanation popup next to the tap. A fresh popup is created
// each time; the previous one (if any) is already dismissed because a non-modal
// popup hides itself on the outside tap that precedes reaching this handler.
func (i *infoIcon) Tapped(e *fyne.PointEvent) {
	if i.message == "" || i.canvas == nil {
		return
	}
	label := widget.NewLabel(i.message)
	label.Wrapping = fyne.TextWrapWord
	content := container.NewPadded(label)
	popup := widget.NewPopUp(content, i.canvas)
	popup.Resize(fyne.NewSize(infoPopupWidth, content.MinSize().Height))
	pos := i.Position().AddXY(i.Size().Width+8, 0)
	if e != nil {
		pos = e.AbsolutePosition.AddXY(12, 12)
	}
	popup.ShowAtPosition(pos)
}

// Cursor shows the pointer cursor so the glyph reads as clickable.
func (i *infoIcon) Cursor() desktop.Cursor {
	return desktop.PointerCursor
}

func (i *infoIcon) CreateRenderer() fyne.WidgetRenderer {
	return &infoIconRenderer{glyph: i.glyph}
}

type infoIconRenderer struct {
	glyph *canvas.Text
}

func (r *infoIconRenderer) Destroy() {}

func (r *infoIconRenderer) Layout(size fyne.Size) {
	glyphSize := r.glyph.MinSize()
	r.glyph.Resize(glyphSize)
	r.glyph.Move(fyne.NewPos((size.Width-glyphSize.Width)/2, (size.Height-glyphSize.Height)/2))
}

func (r *infoIconRenderer) MinSize() fyne.Size {
	return fyne.NewSize(infoIconWidth, infoIconHeight)
}

func (r *infoIconRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.glyph}
}

func (r *infoIconRenderer) Refresh() {
	r.glyph.Refresh()
}
