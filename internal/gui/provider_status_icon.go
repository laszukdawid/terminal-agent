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
	providerStatusWidth  float32 = 28
	providerStatusHeight float32 = 24
)

type providerStatusIcon struct {
	widget.BaseWidget
	canvas  fyne.Canvas
	icon    *canvas.Text
	message string
	popup   *widget.PopUp
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
