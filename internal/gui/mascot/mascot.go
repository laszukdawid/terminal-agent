// Package mascot renders and animates the Terminal Agent sidebar mascot: a
// line-art robot that performs a looping routine while a request is in flight, can
// be grabbed and flung around its pen (a spring-driven, jelly-bodied drag), folds
// its limbs when pressed against a wall, and plays a one-shot reaction when clicked.
//
// The whole thing is one self-contained type, Mascot, driven by a single wall-clock
// ticker. The ticker only runs while there is something to animate (a routine, a
// click reaction, or the drag physics still settling); at true rest it stops and
// the mascot is a single static image, so an idle sidebar costs nothing.
package mascot

import (
	"image/color"
	"math/rand/v2"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// tickDriver is only the cadence at which the forever-repeating fyne animation
// calls back; the routine's timeline is driven by a wall clock so each act can have
// its own duration, so this value is otherwise irrelevant.
const tickDriver = 1 * time.Second

// Mascot is the animated sidebar mascot. Construct it with New, add Object() to the
// sidebar, and call StartRoutine/StopRoutine as requests begin and end.
type Mascot struct {
	stroke color.Color
	image  *canvas.Image
	idle   fyne.Resource
	stage  *stage
	anim   *fyne.Animation

	// Performance scheduler: a routine of distinct acts played one after another off
	// a wall clock. active tracks whether a request is in flight; running whether the
	// ticker is live; routinePlaying whether an act (routine or reaction) is playing
	// (separate from running, which also covers the drag spring settling).
	acts           []act
	actIndex       int
	actStart       time.Time
	active         bool
	running        bool
	routinePlaying bool
	lastName       string

	// frameCache memoizes rendered (undeformed) frames by name. The act timeline is
	// quantized to a fixed frame grid, so that set is finite and recurs; deformed
	// jelly frames vary continuously and are rendered fresh (see applyFrame).
	frameCache map[string]fyne.Resource

	// Click reactions: a one-shot act that takes priority over the routine.
	oneShot      *act
	lastReaction int

	// Grab-and-fling drag physics (see drag.go). pos/vel are the spring's visible
	// offset from home in px; pull is the accumulated cursor drag driving the target;
	// squish is the current wall press fed to the rendered pose. physLast times the
	// integrator; dang holds one inertial dangler per appendage free end.
	stageSize    fyne.Size
	posX, posY   float64
	velX, velY   float64
	pullX, pullY float64
	squishX      float64
	squishY      float64
	dragging     bool
	dragGesture  bool
	physLast     time.Time
	dang         [danglerCount]dangler
}

// New creates a mascot stroked in the given colour, resting.
func New(stroke color.Color) *Mascot {
	m := &Mascot{
		stroke:       stroke,
		acts:         routine(),
		lastReaction: -1, // no reaction played yet; allow any first pick
		frameCache:   make(map[string]fyne.Resource),
	}
	m.idle = renderPose(stroke, restPose(), deform{})
	m.image = canvas.NewImageFromResource(m.idle)
	m.image.FillMode = canvas.ImageFillContain
	m.image.SetMinSize(fyne.NewSize(mascotSize, mascotSize))
	m.stage = newStage(m)
	return m
}

// Object returns the stage widget to place in the sidebar. The mascot roams its
// (invisible) pen within the space the stage is given; a plain click plays a random
// reaction and a drag grabs and flings it.
func (m *Mascot) Object() fyne.CanvasObject { return m.stage }

// StartRoutine begins (or keeps) the performance while a request is in flight. It is
// idempotent: a request already being performed (including one mid click-reaction)
// just re-arms the active flag so the routine keeps chaining.
func (m *Mascot) StartRoutine() {
	if m.image == nil {
		return
	}
	m.active = true
	// Start the act timeline now unless a routine/reaction is already playing (in
	// which case we just re-arm and let it keep chaining). This matters when the
	// ticker is already live only for the drag spring: without it, a stale actStart
	// would make the first act look already finished and be skipped.
	if !m.routinePlaying {
		m.actStart = time.Now()
	}
	m.routinePlaying = true
	m.ensureTicker()
}

// StopRoutine decouples the performance from the request: it never hard-cuts an act.
// It marks the request finished and lets the tick play the current act out to its
// natural end before settling to rest.
func (m *Mascot) StopRoutine() {
	m.active = false
	if !m.running {
		m.showRest()
	}
}

// Stop halts the ticker and returns the mascot to a clean idle state. It is used
// when tearing the mascot down (e.g. before rebuilding the window), but resetting
// all transient play/drag state also leaves the object safe to drive again.
func (m *Mascot) Stop() {
	if m.anim != nil {
		m.anim.Stop()
	}
	m.running = false
	m.routinePlaying = false
	m.active = false
	m.oneShot = nil
	m.dragging = false
	m.dragGesture = false
	m.posX, m.posY, m.velX, m.velY = 0, 0, 0, 0
	m.pullX, m.pullY, m.squishX, m.squishY = 0, 0, 0, 0
	m.dang = [danglerCount]dangler{}
	m.physLast = time.Time{}
}

// ensureTicker starts the forever-repeating wall-clock ticker if it is not already
// running.
func (m *Mascot) ensureTicker() {
	if m.running {
		return
	}
	m.running = true
	if m.anim == nil {
		m.anim = fyne.NewAnimation(tickDriver, m.tick)
		m.anim.Curve = fyne.AnimationLinear
		m.anim.RepeatCount = fyne.AnimationRepeatForever
	}
	m.anim.Start()
}

// playAct interjects a one-shot act (a click reaction) immediately, starting the
// ticker if idle. When the act finishes, the tick resumes the request routine if one
// is active, or settles back to rest otherwise.
func (m *Mascot) playAct(a act) {
	if m.image == nil {
		return
	}
	m.oneShot = &a
	m.routinePlaying = true
	m.actStart = time.Now()
	m.ensureTicker()
}

// playRandomReaction picks a random reaction act (avoiding an immediate repeat) and
// plays it. It backs the mascot's click handler.
func (m *Mascot) playRandomReaction() {
	acts := reactionActs()
	if len(acts) == 0 {
		return
	}
	i := rand.IntN(len(acts))
	if len(acts) > 1 && i == m.lastReaction {
		i = (i + 1) % len(acts)
	}
	m.lastReaction = i
	m.playAct(acts[i])
}

// showRest displays the still resting pose.
func (m *Mascot) showRest() {
	if m.image == nil {
		return
	}
	m.image.Resource = m.idle
	m.image.Refresh()
	m.lastName = m.idle.Name()
}

// settleToRest ends the routine and the drag physics: it stops the ticker (safe to
// call from within the tick in fyne 2.7.x), snaps the mascot back to its home
// position, and restores the resting pose.
func (m *Mascot) settleToRest() {
	m.running = false
	m.routinePlaying = false
	if m.anim != nil {
		m.anim.Stop()
	}
	// Reset the drag spring to home and re-place the image there. The physics is
	// already at rest whenever the ticker stops, so this only formalises it.
	m.posX, m.posY, m.velX, m.velY = 0, 0, 0, 0
	m.squishX, m.squishY, m.pullX, m.pullY = 0, 0, 0, 0
	m.dang = [danglerCount]dangler{}
	m.physLast = time.Time{}
	m.layoutImage()
	m.showRest()
}

// tick advances everything driven off the wall clock each frame: the grab physics
// (mascot position and wall squish) plus the act routine when one is playing. The
// ticker keeps running while either the routine or the drag spring still needs
// frames, and stops once both are at rest so idle costs nothing.
func (m *Mascot) tick(float32) {
	if !m.running || m.image == nil {
		return
	}
	now := time.Now()
	dt := m.frameDT(now)

	// Move and (if pinned to a wall) squish the mascot. This runs every tick,
	// independent of whatever pose the routine is drawing.
	m.stepPhysics(dt)

	// The base pose is the current routine/reaction frame while one plays, otherwise
	// the mascot simply rests.
	base := restPose()
	if m.routinePlaying {
		base = m.advanceRoutine(now)
	}

	// The jelly-bot dynamics deform that pose: the limbs fold against a pressed wall
	// and lag the motion (drag, act, or both) with their own springs. One system for
	// all interactions.
	d := m.stepDynamics(base, dt)
	m.applyFrame(base, d)

	// Stop only once the routine, the drag spring, and the jelly dynamics are all at
	// rest, so a fling released after the routine ends still gets to spring and jiggle
	// home before the ticker halts.
	if !m.routinePlaying && m.physicsSettled() && m.dynamicsSettled() {
		m.settleToRest()
	}
}

// advanceRoutine returns the routine/reaction pose for this tick, advancing the act
// scheduler and chaining acts off the wall clock. When the performance reaches its
// natural end it clears routinePlaying and returns the rest pose; it never stops the
// ticker itself, leaving that to tick so the drag spring can keep animating.
func (m *Mascot) advanceRoutine(now time.Time) pose {
	// A click reaction is a one-shot that takes priority over the request routine.
	a := m.oneShot
	if a == nil {
		if len(m.acts) == 0 {
			m.routinePlaying = false
			return restPose()
		}
		cur := m.acts[m.actIndex]
		a = &cur
	}

	elapsed := now.Sub(m.actStart).Seconds()
	if elapsed >= a.dur {
		switch {
		case m.oneShot != nil:
			// The click reaction finished.
			m.oneShot = nil
			if !m.active || len(m.acts) == 0 {
				m.routinePlaying = false
				return restPose()
			}
			// Resume the request routine where it left off.
			m.actStart = now
			cur := m.acts[m.actIndex]
			a = &cur
			elapsed = 0
		case !m.active:
			// Request finished and the act has played out.
			m.routinePlaying = false
			return restPose()
		default:
			// Chain to the next act in the routine.
			m.actIndex = (m.actIndex + 1) % len(m.acts)
			m.actStart = now
			cur := m.acts[m.actIndex]
			a = &cur
			elapsed = 0
		}
	}
	return a.fn(quantizeTime(elapsed))
}

// applyFrame renders the pose plus its deformation and swaps it in, skipping the
// refresh when the quantized frame is unchanged so a slow or still moment costs
// nothing.
func (m *Mascot) applyFrame(p pose, d deform) {
	// Snap to the cache grid first, then both name and render use the snapped pose,
	// so a given name always renders identical SVG bytes.
	p = quantizePose(p)
	name := frameName(m.stroke, p, d)
	if name == m.lastName {
		return
	}
	m.lastName = name
	// Reuse the rendered frame if we have drawn this pose before; the quantized
	// timeline makes frames recur, so this stays bounded. Only the undeformed
	// routine/rest frames are memoised: they form a small recurring set. Jelly frames
	// vary continuously and only occur transiently while things are in motion, so
	// caching them would let the map grow without bound; render those fresh and lean
	// on Fyne's own name-keyed raster cache (bounded by the deform quantization).
	cacheable := d.isZero()
	var res fyne.Resource
	if cacheable {
		res = m.frameCache[name]
	}
	if res == nil {
		res = renderPose(m.stroke, p, d)
		if cacheable && m.frameCache != nil {
			m.frameCache[name] = res
		}
	}
	m.image.Resource = res
	m.image.Refresh()
}
