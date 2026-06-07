package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	navRowHeight  float32 = 30
	navIconLeft   float32 = 14
	navMarkerW    float32 = 3
	statusDotSize float32 = 9
)

// brandSeparator is a 1px rule in the muted panel-border color. It reads as a
// horizontal line inside vertical containers and as a vertical divider when
// placed in a Border side slot.
func brandSeparator() fyne.CanvasObject  { return widget.NewSeparator() }
func brandVSeparator() fyne.CanvasObject { return widget.NewSeparator() }

// brandSectionLabel renders an uppercase green section heading
// ("ASK THE TERMINAL AGENT", "RESPONSE") in the terminal-native style.
func brandSectionLabel(text string) *canvas.Text {
	t := canvas.NewText(text, brandAccentGreen)
	t.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	t.TextSize = theme.TextSize() - 1
	return t
}

// borderedBox wraps content in a thin-bordered, slightly-rounded panel filled
// with the brand panel color. The brand favors crisp bordered panels over
// heavy shadows; callers pass the stroke color to distinguish the emphasized
// input field (bright green border) from calmer output/mascot panels.
func borderedBox(content fyne.CanvasObject, stroke color.Color) *fyne.Container {
	rect := canvas.NewRectangle(brandPanel)
	rect.StrokeColor = stroke
	rect.StrokeWidth = 1
	rect.CornerRadius = 5
	return container.NewStack(rect, content)
}

// newStatusDot is the small filled "●" indicator used by the model pill and the
// connection status block.
func newStatusDot(fill color.Color) fyne.CanvasObject {
	dot := canvas.NewCircle(fill)
	return container.NewCenter(container.NewGridWrap(fyne.NewSize(statusDotSize, statusDotSize), dot))
}

// newCommandButton is a lightweight, line-icon labelled action (COPY, EXPORT)
// that sits under the response panel. It uses low importance so it reads as a
// terminal output action rather than a document-toolbar button.
func newCommandButton(label, iconPath string, onTap func()) *widget.Button {
	btn := widget.NewButtonWithIcon(label, lineIcon("cmd-"+label, iconPath, brandMutedGreen), onTap)
	btn.Importance = widget.LowImportance
	return btn
}

// brandActionDivider is the muted "|" separator shown between COPY and EXPORT.
func brandActionDivider() fyne.CanvasObject {
	t := canvas.NewText("|", brandSecondaryText)
	t.TextStyle = fyne.TextStyle{Monospace: true}
	return container.NewCenter(t)
}

// fixedWidthLayout forces its content to a fixed width (used for the sidebar
// rail) while letting height flow from the parent Border slot.
type fixedWidthLayout struct {
	width float32
}

func (f *fixedWidthLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	height := float32(0)
	for _, o := range objects {
		if o.Visible() {
			height = max(height, o.MinSize().Height)
		}
	}
	return fyne.NewSize(f.width, height)
}

func (f *fixedWidthLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Resize(fyne.NewSize(f.width, size.Height))
		o.Move(fyne.NewPos(0, 0))
	}
}

// navRow is a sidebar navigation entry: an optional active green left marker, a
// line icon, and a label. The active row gets a subtle elevated background and
// green text/icon; inactive rows highlight on hover.
type navRow struct {
	widget.BaseWidget
	label   string
	active  bool
	onTap   func()
	hovered bool

	bg     *canvas.Rectangle
	marker *canvas.Rectangle
	icon   *canvas.Image
	text   *canvas.Text
}

func newNavRow(label, iconPath string, active bool, onTap func()) *navRow {
	n := &navRow{label: label, active: active, onTap: onTap}

	textColor := brandSecondaryText
	if active {
		textColor = brandAccentGreen
	}

	n.bg = canvas.NewRectangle(color.Transparent)
	n.bg.CornerRadius = 4
	if active {
		n.bg.FillColor = brandElevatedPanel
	}

	n.marker = canvas.NewRectangle(color.Transparent)
	if active {
		n.marker.FillColor = brandAccentGreen
	}

	n.icon = canvas.NewImageFromResource(lineIcon("nav-"+label, iconPath, textColor))
	n.icon.FillMode = canvas.ImageFillContain

	n.text = canvas.NewText(label, textColor)
	n.text.TextStyle = fyne.TextStyle{Monospace: true, Bold: active}
	n.text.TextSize = theme.TextSize() - 1

	n.ExtendBaseWidget(n)
	return n
}

func (n *navRow) Tapped(*fyne.PointEvent) {
	if n.onTap != nil {
		n.onTap()
	}
}

func (n *navRow) MouseIn(*desktop.MouseEvent) {
	if n.active {
		return
	}
	n.hovered = true
	n.bg.FillColor = brandElevatedPanel
	n.bg.Refresh()
}

func (n *navRow) MouseMoved(*desktop.MouseEvent) {}

func (n *navRow) MouseOut() {
	if n.active {
		return
	}
	n.hovered = false
	n.bg.FillColor = color.Transparent
	n.bg.Refresh()
}

func (n *navRow) Cursor() desktop.Cursor {
	if n.onTap != nil {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

func (n *navRow) CreateRenderer() fyne.WidgetRenderer {
	return &navRowRenderer{row: n, objects: []fyne.CanvasObject{n.bg, n.marker, n.icon, n.text}}
}

type navRowRenderer struct {
	row     *navRow
	objects []fyne.CanvasObject
}

func (r *navRowRenderer) Destroy() {}

func (r *navRowRenderer) Layout(size fyne.Size) {
	r.row.bg.Resize(size)
	r.row.bg.Move(fyne.NewPos(0, 0))

	r.row.marker.Resize(fyne.NewSize(navMarkerW, size.Height))
	r.row.marker.Move(fyne.NewPos(0, 0))

	iconY := (size.Height - navIconSize) / 2
	r.row.icon.Resize(fyne.NewSize(navIconSize, navIconSize))
	r.row.icon.Move(fyne.NewPos(navIconLeft, iconY))

	textSize := r.row.text.MinSize()
	r.row.text.Resize(textSize)
	r.row.text.Move(fyne.NewPos(navIconLeft+navIconSize+10, (size.Height-textSize.Height)/2))
}

func (r *navRowRenderer) MinSize() fyne.Size {
	textSize := r.row.text.MinSize()
	return fyne.NewSize(navIconLeft+navIconSize+10+textSize.Width+8, navRowHeight)
}

func (r *navRowRenderer) Objects() []fyne.CanvasObject { return r.objects }

func (r *navRowRenderer) Refresh() {
	r.row.bg.Refresh()
	r.row.marker.Refresh()
	r.row.icon.Refresh()
	r.row.text.Refresh()
}
