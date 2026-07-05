package mascot

import (
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Grab-and-fling drag physics for the sidebar mascot. The mascot lives on a stage
// that fills the sidebar's open space; grabbing it pulls it toward the cursor
// against a spring, it can never leave its pen, and pulling past a wall folds the
// limbs pressed against it back toward the body instead of dragging them through.
const (
	// Spring pulling the visible mascot toward its target. The damping ratio is a
	// touch below 1 (damping / (2*sqrt(stiffness)) ~= 0.8) so a release bounces home
	// with a little life rather than gliding in dead-flat.
	dragStiffness = 220.0
	dragDamping   = 24.0
	dragMaxDT     = 0.05 // clamp the integration step so a stall can't explode it

	// Rubber-band resistance. Inside the soft zone the mascot follows the cursor
	// directly; past it, motion toward the wall decays so the wall is approached but
	// never crossed, and the leftover pull becomes squish.
	dragSoftFraction = 0.55

	// Wall squish. The cursor must pull dragSquishRange pixels past the wall for full
	// press; a limb pressed against the wall then folds back toward the body by up to
	// foldStrength of however far it stuck out (see foldLimb).
	dragSquishRange = 42.0
	dragMaxSquish   = 0.38
	foldStrength    = 0.9   // fraction of a limb's protrusion folded away at full press
	squishResponse  = 16.0  // how fast the press eases in and out
	squishEps       = 0.005 // below this the press reads as none

	// Below these offset/velocity thresholds (and with no squish) a released mascot
	// is considered home and the physics can stop.
	settlePos = 0.5
	settleVel = 3.0

	// The drawn robot's visible half-extents and centre offset within its cell, in
	// px (the cell is mostly transparent headroom). Derived from the pose geometry:
	// the SVG's 96px canvas is contained in the cell, so one canvas unit is
	// mascotSize/mascotCanvas px.
	visHalfW      = (mascotVisRightC - mascotVisLeftC) / 2 * mascotSize / mascotCanvas
	visHalfH      = (mascotVisBottomC - mascotVisTopC) / 2 * mascotSize / mascotCanvas
	visCenterOffX = mascotVisCenterXC*mascotSize/mascotCanvas - mascotSize/2
	visCenterOffY = mascotVisCenterYC*mascotSize/mascotCanvas - mascotSize/2

	// penMargin is the gap (px) between the drawn robot and its boundary pen; it is
	// also how far the robot can travel from home before it meets the wall. (Travel
	// is still capped so the mascot cell never leaves a narrow stage.)
	penMargin = 42.0
)

// stage is the free-roaming area that hosts the mascot in the sidebar. It fills the
// space between the nav and the status card and owns the mascot image, positioning
// it via the drag physics. Only the mascot itself is interactive: clicking it plays
// a random reaction and dragging it grabs and flings it, while clicks on the
// surrounding empty space are ignored. It tracks the hover point so the grab cursor
// only shows over the mascot, not the whole stage.
type stage struct {
	widget.BaseWidget
	m        *Mascot
	hoverPos fyne.Position
	hovering bool
}

func newStage(m *Mascot) *stage {
	s := &stage{m: m}
	s.ExtendBaseWidget(s)
	return s
}

// Tapped plays a random reaction, but only when the click landed on the mascot.
func (s *stage) Tapped(e *fyne.PointEvent) {
	if s.m.onMascot(e.Position) {
		s.m.playRandomReaction()
	}
}

// Dragged feeds the drag physics. e.Position has already advanced by e.Dragged from
// where the press began, so the grab origin (used to decide whether the gesture
// started on the mascot) is e.Position minus that delta. This keeps a fast opening
// flick that begins on the mascot from being dropped because its first reported
// position already jumped off the box.
func (s *stage) Dragged(e *fyne.DragEvent) {
	origin := fyne.NewPos(e.Position.X-e.Dragged.DX, e.Position.Y-e.Dragged.DY)
	s.m.drag(origin, float64(e.Dragged.DX), float64(e.Dragged.DY))
}

// DragEnd releases the mascot so the spring pulls it home.
func (s *stage) DragEnd() { s.m.dragEnd() }

// MouseIn/MouseMoved/MouseOut track the hover point so Cursor can tell whether the
// pointer is over the mascot.
func (s *stage) MouseIn(e *desktop.MouseEvent) {
	s.hovering = true
	s.hoverPos = e.Position
}
func (s *stage) MouseMoved(e *desktop.MouseEvent) { s.hoverPos = e.Position }
func (s *stage) MouseOut()                        { s.hovering = false }

// Cursor shows the grab hand only while the pointer is over the mascot.
func (s *stage) Cursor() desktop.Cursor {
	if s.hovering && s.m.onMascot(s.hoverPos) {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

func (s *stage) CreateRenderer() fyne.WidgetRenderer {
	return &stageRenderer{stage: s}
}

// stageRenderer draws the mascot image. On every (re)layout it records the stage
// size and re-places the image, so a window resize does not snap the mascot away
// from where the spring has it.
type stageRenderer struct {
	stage *stage
}

func (r *stageRenderer) Layout(size fyne.Size) {
	r.stage.m.stageSize = size
	r.stage.m.layoutImage()
}

func (r *stageRenderer) MinSize() fyne.Size {
	return fyne.NewSize(mascotSize, mascotSize)
}

func (r *stageRenderer) Refresh() {
	if img := r.stage.m.image; img != nil {
		img.Refresh()
	}
}

func (r *stageRenderer) Objects() []fyne.CanvasObject {
	if img := r.stage.m.image; img != nil {
		return []fyne.CanvasObject{img}
	}
	return nil
}

func (r *stageRenderer) Destroy() {}

// onMascot reports whether a point (in stage coordinates) falls within the mascot
// image's current box, so only clicks on the mascot grab it.
func (m *Mascot) onMascot(pos fyne.Position) bool {
	if m.image == nil {
		return false
	}
	homeX := (float64(m.stageSize.Width) - float64(mascotSize)) / 2
	homeY := (float64(m.stageSize.Height) - float64(mascotSize)) / 2
	x0, y0 := homeX+m.posX, homeY+m.posY
	x, y := float64(pos.X), float64(pos.Y)
	return x >= x0 && x <= x0+float64(mascotSize) && y >= y0 && y <= y0+float64(mascotSize)
}

// drag advances a drag gesture. origin is the point where the gesture began (stage
// coordinates); the gesture only grabs the mascot if that origin landed on it. Once
// grabbed it accumulates the cursor delta into the pull, seeded from the mascot's
// current offset so re-grabbing it mid-spring does not snap it to the cursor.
func (m *Mascot) drag(origin fyne.Position, dx, dy float64) {
	if m.image == nil {
		return
	}
	if !m.dragGesture {
		m.dragGesture = true
		if m.onMascot(origin) {
			m.dragging = true
			m.pullX = m.posX
			m.pullY = m.posY
			m.ensureTicker()
		}
	}
	if !m.dragging {
		return // the gesture started off the mascot; ignore it
	}
	m.pullX += dx
	m.pullY += dy
	m.ensureTicker()
}

// dragEnd ends the current gesture. If it had grabbed the mascot, the pull target
// collapses to home and the tick keeps running until the spring settles.
func (m *Mascot) dragEnd() {
	m.dragGesture = false
	if !m.dragging {
		return
	}
	m.dragging = false
	m.pullX = 0
	m.pullY = 0
	m.ensureTicker()
}

// pen describes the boundary the mascot roams: halfW/halfH are the pen's
// half-extents (px) around the drawn robot, and roamX/roamY are how far the robot
// may travel from home before its figure meets the wall. The pen hugs the drawn
// robot (a little larger than it), and the travel is capped so the cell can never
// leave the stage and spill over the neighbouring panels.
func (m *Mascot) pen() (halfW, halfH, roamX, roamY float64) {
	maxRoamX := float64(m.stageSize.Width)/2 - float64(mascotSize)/2
	maxRoamY := float64(m.stageSize.Height)/2 - float64(mascotSize)/2
	roamX = math.Max(0, math.Min(penMargin, maxRoamX))
	roamY = math.Max(0, math.Min(penMargin, maxRoamY))
	halfW = float64(visHalfW) + roamX
	halfH = float64(visHalfH) + roamY
	return
}

// roamLimits are the maximum offsets (px) from home in each direction.
func (m *Mascot) roamLimits() (float64, float64) {
	_, _, roamX, roamY := m.pen()
	return roamX, roamY
}

// frameDT returns the seconds since the previous tick, clamped so a stall can't
// explode the integrators, and 0 on the first tick of a run.
func (m *Mascot) frameDT(now time.Time) float64 {
	dt := 0.0
	if !m.physLast.IsZero() {
		dt = now.Sub(m.physLast).Seconds()
	}
	m.physLast = now
	if dt < 0 {
		dt = 0
	}
	if dt > dragMaxDT {
		dt = dragMaxDT
	}
	return dt
}

// stepPhysics advances the position spring one frame: it pulls the visible offset
// toward the (rubber-banded, wall-clamped) target, updates the wall squish, and
// re-places the mascot image.
func (m *Mascot) stepPhysics(dt float64) {
	limX, limY := m.roamLimits()

	var tgtX, tgtY, overX, overY float64
	if m.dragging {
		tgtX, overX = rubberBand(m.pullX, limX)
		tgtY, overY = rubberBand(m.pullY, limY)
	}

	// Spring the visible position toward the target.
	m.velX += (dragStiffness*(tgtX-m.posX) - dragDamping*m.velX) * dt
	m.velY += (dragStiffness*(tgtY-m.posY) - dragDamping*m.velY) * dt
	m.posX += m.velX * dt
	m.posY += m.velY * dt

	// The walls are solid: a spring overshoot must not carry the mascot through.
	m.posX, m.velX = clampWall(m.posX, m.velX, limX)
	m.posY, m.velY = clampWall(m.posY, m.velY, limY)

	// Ease the wall squish in while pulling past a wall and out on release.
	var sqTgtX, sqTgtY float64
	if m.dragging {
		sqTgtX = math.Copysign(squishAmount(overX), m.pullX)
		sqTgtY = math.Copysign(squishAmount(overY), m.pullY)
	}
	k := math.Min(1, dt*squishResponse)
	m.squishX += (sqTgtX - m.squishX) * k
	m.squishY += (sqTgtY - m.squishY) * k
	if math.Abs(m.squishX) < squishEps {
		m.squishX = 0
	}
	if math.Abs(m.squishY) < squishEps {
		m.squishY = 0
	}

	if m.physicsSettled() {
		m.posX, m.posY, m.velX, m.velY = 0, 0, 0, 0
	}

	m.layoutImage()
}

// physicsSettled reports whether a released mascot has come to rest at home with no
// squish, so the ticker can stop.
func (m *Mascot) physicsSettled() bool {
	if m.dragging {
		return false
	}
	return math.Abs(m.posX) < settlePos &&
		math.Abs(m.posY) < settlePos &&
		math.Abs(m.velX) < settleVel &&
		math.Abs(m.velY) < settleVel &&
		m.squishX == 0 && m.squishY == 0
}

// layoutImage sizes the mascot and places it at home plus the current drag offset.
// It is a no-op move when the position has not changed so still frames cost nothing.
func (m *Mascot) layoutImage() {
	img := m.image
	if img == nil {
		return
	}
	if s := img.Size(); s.Width != mascotSize || s.Height != mascotSize {
		img.Resize(fyne.NewSize(mascotSize, mascotSize))
	}
	homeX := (float64(m.stageSize.Width) - float64(mascotSize)) / 2
	homeY := (float64(m.stageSize.Height) - float64(mascotSize)) / 2
	pos := fyne.NewPos(float32(homeX+m.posX), float32(homeY+m.posY))
	if img.Position() != pos {
		img.Move(pos)
	}
}

// clampWall keeps pos within [-limit, limit] and kills any velocity still pushing
// it into the wall, so hitting an edge reads as a solid stop.
func clampWall(pos, vel, limit float64) (float64, float64) {
	switch {
	case pos > limit:
		pos = limit
		if vel > 0 {
			vel = 0
		}
	case pos < -limit:
		pos = -limit
		if vel < 0 {
			vel = 0
		}
	}
	return pos, vel
}

// rubberBand maps the raw accumulated pull to the visible offset and the leftover
// overshoot. Within the soft zone the offset equals the pull (direct follow);
// beyond it the remaining travel to the wall is approached asymptotically, so the
// offset never reaches past limit and the returned overshoot (>= 0) is how far the
// cursor pulled beyond what the mascot could move.
func rubberBand(pull, limit float64) (pos, over float64) {
	if limit <= 0 {
		return 0, 0
	}
	a := math.Abs(pull)
	sign := math.Copysign(1, pull)
	soft := limit * dragSoftFraction
	if a <= soft {
		return pull, 0
	}
	span := limit - soft
	extra := a - soft
	travelled := span * (1 - math.Exp(-extra/span))
	return sign * (soft + travelled), extra - travelled
}

// squishAmount converts a pull overshoot (px past the wall) into a compression
// fraction, ramping to dragMaxSquish over dragSquishRange.
func squishAmount(overshoot float64) float64 {
	if overshoot <= 0 {
		return 0
	}
	f := overshoot / dragSquishRange
	if f > 1 {
		f = 1
	}
	return f * dragMaxSquish
}

// --- jelly-bot secondary motion --------------------------------------------

// Appendage dangler tuning. Each free end is a point mass on a spring toward its
// pose-commanded position; the body's motion drags it, so it lags and whips.
const (
	danglerStiffness  = 300.0 // spring back to the commanded position
	danglerDamping    = 20.0  // ~underdamped (zeta ~0.58): a couple of bounces, then settle
	danglerMaxDeflect = 14.0  // cap the lag (body-grid units) so limbs can't fly off
	danglerRestEps    = 0.05  // below this offset+speed a dangler snaps to rest
	danglerRestVel    = 0.4
	danglerCount      = 5
)

// Indices into Mascot.dang, one per appendage free end.
const (
	dangBulb = iota
	dangHandL
	dangHandR
	dangFootL
	dangFootR
)

// dangler is one appendage free end (bulb, hand, or foot): a point mass on a spring
// toward its pose-commanded target, tracked as the lag offset from that target plus
// the mass's own world velocity (body-grid units).
type dangler struct {
	ox, oy float64 // current lag offset from the target
	vx, vy float64 // the mass's world velocity
	tx, ty float64 // previous target position
	seeded bool
}

// step advances the mass one frame: the spring pulls its world velocity toward the
// target while damping bleeds momentum, and the target's displacement this frame
// shifts the lag. So the free end trails while the body moves steadily, overshoots
// and whips when it starts/stops/slams, then rings down to rest. It uses the target
// displacement (not its velocity), so it stays stable even when two frames land
// microseconds apart. Nothing here knows which interaction moved the body.
func (d *dangler) step(targetX, targetY, dt float64) {
	if !d.seeded || dt <= 0 {
		d.tx, d.ty, d.seeded = targetX, targetY, true
		return
	}
	dx, dy := targetX-d.tx, targetY-d.ty
	d.vx += (-danglerStiffness*d.ox - danglerDamping*d.vx) * dt
	d.vy += (-danglerStiffness*d.oy - danglerDamping*d.vy) * dt
	d.ox += d.vx*dt - dx
	d.oy += d.vy*dt - dy
	if mag := math.Hypot(d.ox, d.oy); mag > danglerMaxDeflect {
		s := danglerMaxDeflect / mag
		d.ox, d.oy = d.ox*s, d.oy*s
	}
	if math.Abs(d.ox) < danglerRestEps && math.Abs(d.oy) < danglerRestEps &&
		math.Abs(d.vx) < danglerRestVel && math.Abs(d.vy) < danglerRestVel {
		d.ox, d.oy, d.vx, d.vy = 0, 0, 0, 0
	}
	d.tx, d.ty = targetX, targetY
}

func (d *dangler) atRest() bool {
	return d.ox == 0 && d.oy == 0 && d.vx == 0 && d.vy == 0
}

// stepDynamics advances the jelly physics one frame and returns the frame's
// deformation. The body squash comes straight from the position spring's wall
// press; each appendage dangler is driven by the motion of its own commanded
// position (the pose target plus the whole-body translation from acts and drag), so
// acts, flings, and wall slams all feed the same springs.
func (m *Mascot) stepDynamics(p pose, dt float64) deform {
	// Whole-body translation the appendages ride, in body-grid units: the drag
	// offset (px -> grid) plus the act's translation. Rotation is intentionally left
	// out of the driver so a spin can't fling the limbs.
	pxToGrid := mascotCanvas / float64(mascotSize)
	bodyX := m.posX*pxToGrid + p.tx
	bodyY := m.posY*pxToGrid + p.ty

	bulb, handL, handR, footL, footR := appendages(p, m.squishX, m.squishY)
	targets := [danglerCount]vec2{bulb, handL, handR, footL, footR}
	for i := range m.dang {
		m.dang[i].step(bodyX+targets[i].x, bodyY+targets[i].y, dt)
	}

	return deform{
		squashX: m.squishX,
		squashY: m.squishY,
		bulb:    vec2{m.dang[dangBulb].ox, m.dang[dangBulb].oy},
		handL:   vec2{m.dang[dangHandL].ox, m.dang[dangHandL].oy},
		handR:   vec2{m.dang[dangHandR].ox, m.dang[dangHandR].oy},
		footL:   vec2{m.dang[dangFootL].ox, m.dang[dangFootL].oy},
		footR:   vec2{m.dang[dangFootR].ox, m.dang[dangFootR].oy},
	}
}

// dynamicsSettled reports whether every appendage lag has died out, so the ticker
// can stop and idle stays free.
func (m *Mascot) dynamicsSettled() bool {
	for i := range m.dang {
		if !m.dang[i].atRest() {
			return false
		}
	}
	return true
}
