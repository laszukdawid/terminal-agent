package gui

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	navRowHeight  float32 = 30
	navIconLeft   float32 = 14
	navMarkerW    float32 = 3
	statusDotSize float32 = 9

	// panelPadV/panelPadH are the breathing room between a bordered panel's
	// edge and its content. The brand favors generous, calm spacing inside
	// crisp bordered panels rather than content hugging the border.
	panelPadV float32 = 10
	panelPadH float32 = 14

	// listenLetterGap is the slight tracking applied between letters of the
	// LISTEN caption. Monospace can't sub-divide a cell, so the caption is laid
	// out letter-by-letter with this pixel gap for fine control.
	listenLetterGap float32 = 3
)

// hStrut is a fixed-width transparent spacer for putting deliberate horizontal
// space between inline elements (e.g. the input hint and the SEND button).
func hStrut(w float32) fyne.CanvasObject {
	return container.NewGridWrap(fyne.NewSize(w, 1), canvas.NewRectangle(color.Transparent))
}

// vCenter vertically centers content at its natural height inside a taller
// row, so it gets even space above and below instead of stretching to fill.
func vCenter(content fyne.CanvasObject) fyne.CanvasObject {
	return container.NewVBox(layout.NewSpacer(), content, layout.NewSpacer())
}

// letterRowLayout lays objects left-to-right with a fixed pixel gap between
// them and vertically centers each. It backs letter-spaced captions where a
// full monospace space would be too wide.
type letterRowLayout struct {
	gap float32
}

func (l *letterRowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var width, height float32
	for i, o := range objects {
		s := o.MinSize()
		if i > 0 {
			width += l.gap
		}
		width += s.Width
		height = max(height, s.Height)
	}
	return fyne.NewSize(width, height)
}

func (l *letterRowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	var x float32
	for i, o := range objects {
		s := o.MinSize()
		if i > 0 {
			x += l.gap
		}
		o.Resize(s)
		o.Move(fyne.NewPos(x, (size.Height-s.Height)/2))
		x += s.Width
	}
}

// letterRow renders text as individual letters with letterLetterGap tracking.
func letterRow(text string, col color.Color, size float32) *fyne.Container {
	objects := make([]fyne.CanvasObject, 0, len(text))
	for _, r := range text {
		t := canvas.NewText(string(r), col)
		t.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
		t.TextSize = size
		objects = append(objects, t)
	}
	return container.New(&letterRowLayout{gap: listenLetterGap}, objects...)
}

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
	padded := container.New(layout.NewCustomPaddedLayout(panelPadV, panelPadV, panelPadH, panelPadH), content)
	return container.NewStack(rect, padded)
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

const (
	// dataLane visuals: a faint dotted "wire" with a bold packet that travels
	// along it during a request. Dots near the packet swell and brighten.
	laneMargin     float32 = 6
	laneMinWidth   float32 = 90
	laneMinHeight  float32 = 16
	laneDotBaseR   float32 = 1.4
	laneDotMidR    float32 = 2.0
	laneDotBoldR   float32 = 2.9
	laneDotMidDist float32 = 14
	lanePacketSize float32 = 8
)

// dataLane draws a row of dots (the wire between the agent and the host) and a
// bold packet that can be moved along it to visualize data transfer. When idle
// it is just a faint evenly-spaced dotted rule.
type dataLane struct {
	widget.BaseWidget
	dots     []*canvas.Circle
	packet   *canvas.Rectangle
	progress float32
	active   bool
	w, h     float32
}

func newDataLane(dotCount int) *dataLane {
	l := &dataLane{dots: make([]*canvas.Circle, dotCount)}
	for i := range l.dots {
		l.dots[i] = canvas.NewCircle(brandBorderBright)
	}
	l.packet = canvas.NewRectangle(brandAccentGreen)
	l.packet.CornerRadius = 1.5
	l.packet.Hide()
	l.ExtendBaseWidget(l)
	return l
}

// SetProgress moves the packet to progress in [0,1] along the lane and lights
// up nearby dots. It marks the lane active so the packet is shown.
func (l *dataLane) SetProgress(progress float32) {
	l.progress = progress
	l.active = true
	l.relayout()
	l.Refresh()
}

// SetIdle hides the packet and restores the faint dotted rule.
func (l *dataLane) SetIdle() {
	l.active = false
	l.relayout()
	l.Refresh()
}

func (l *dataLane) relayout() {
	n := len(l.dots)
	if n == 0 || l.w <= 0 {
		return
	}
	span := l.w - 2*laneMargin
	if span < 0 {
		span = 0
	}
	cy := l.h / 2
	head := laneMargin + l.progress*span

	for i, dot := range l.dots {
		var x float32 = laneMargin
		if n > 1 {
			x += span * float32(i) / float32(n-1)
		}
		r, col := laneDotStyle(absF(x-head), l.active)
		dot.FillColor = col
		dot.Resize(fyne.NewSize(2*r, 2*r))
		dot.Move(fyne.NewPos(x-r, cy-r))
		dot.Refresh()
	}

	if l.active {
		l.packet.Resize(fyne.NewSize(lanePacketSize, lanePacketSize))
		l.packet.Move(fyne.NewPos(head-lanePacketSize/2, cy-lanePacketSize/2))
		l.packet.Show()
	} else {
		l.packet.Hide()
	}
	l.packet.Refresh()
}

// laneDotStyle returns the radius and color for a dot at distPx pixels from the
// packet head; dots near the packet are larger and brighter.
func laneDotStyle(distPx float32, active bool) (float32, color.Color) {
	if !active {
		return laneDotBaseR, brandBorderBright
	}
	switch {
	case distPx <= lanePacketSize:
		return laneDotBoldR, brandAccentGreen
	case distPx <= laneDotMidDist:
		return laneDotMidR, brandMutedGreen
	default:
		return laneDotBaseR, brandBorderBright
	}
}

func absF(v float32) float32 { return float32(math.Abs(float64(v))) }

func (l *dataLane) CreateRenderer() fyne.WidgetRenderer {
	objects := make([]fyne.CanvasObject, 0, len(l.dots)+1)
	for _, dot := range l.dots {
		objects = append(objects, dot)
	}
	objects = append(objects, l.packet)
	return &dataLaneRenderer{lane: l, objects: objects}
}

type dataLaneRenderer struct {
	lane    *dataLane
	objects []fyne.CanvasObject
}

func (r *dataLaneRenderer) Destroy() {}

func (r *dataLaneRenderer) Layout(size fyne.Size) {
	r.lane.w = size.Width
	r.lane.h = size.Height
	r.lane.relayout()
}

func (r *dataLaneRenderer) MinSize() fyne.Size { return fyne.NewSize(laneMinWidth, laneMinHeight) }

func (r *dataLaneRenderer) Objects() []fyne.CanvasObject { return r.objects }

func (r *dataLaneRenderer) Refresh() { canvas.Refresh(r.lane) }

const (
	// Host (receiving) node geometry and receive reaction. The node sits in a
	// fixed box large enough for the icon to swell and shake on packet arrival.
	hostNodeSize         float32 = 40
	hostIconBase         float32 = 30
	hostIconGrow         float32 = 5
	hostShakeAmp         float32 = 2.5
	hostActiveLevelStart float32 = 0.3
)

// hostNode is the receiving host/server. It rests at a fixed size and, as a
// packet arrives, swells slightly and jitters horizontally (a "shake"),
// returning to rest as the packet leaves.
type hostNode struct {
	widget.BaseWidget
	image     *canvas.Image
	idleRes   fyne.Resource
	activeRes fyne.Resource
	size      float32
	dx        float32
	w, h      float32
}

func newHostNode(idle, active fyne.Resource) *hostNode {
	n := &hostNode{idleRes: idle, activeRes: active, size: hostIconBase}
	n.image = canvas.NewImageFromResource(idle)
	n.image.FillMode = canvas.ImageFillContain
	n.ExtendBaseWidget(n)
	return n
}

// SetState reacts to a packet: level in [0,1] is arrival proximity (drives size
// and shake amplitude), and shake in [-1,1] is the oscillation phase.
func (n *hostNode) SetState(level, shake float32) {
	if level < 0 {
		level = 0
	} else if level > 1 {
		level = 1
	}
	n.size = hostIconBase + level*hostIconGrow
	n.dx = shake * hostShakeAmp * level
	res := n.idleRes
	if level >= hostActiveLevelStart {
		res = n.activeRes
	}
	if n.image.Resource != res {
		n.image.Resource = res
	}
	n.reposition()
	n.Refresh()
}

func (n *hostNode) reposition() {
	if n.w <= 0 {
		return
	}
	n.image.Resize(fyne.NewSize(n.size, n.size))
	n.image.Move(fyne.NewPos((n.w-n.size)/2+n.dx, (n.h-n.size)/2))
}

func (n *hostNode) CreateRenderer() fyne.WidgetRenderer {
	return &hostNodeRenderer{node: n}
}

type hostNodeRenderer struct {
	node *hostNode
}

func (r *hostNodeRenderer) Destroy() {}

func (r *hostNodeRenderer) Layout(size fyne.Size) {
	r.node.w = size.Width
	r.node.h = size.Height
	r.node.reposition()
}

func (r *hostNodeRenderer) MinSize() fyne.Size { return fyne.NewSize(hostNodeSize, hostNodeSize) }

func (r *hostNodeRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.node.image} }

func (r *hostNodeRenderer) Refresh() { canvas.Refresh(r.node) }
