package mascot

import (
	"image/color"
	"math"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// testStroke is an arbitrary mascot colour for render/identity tests.
var testStroke = color.NRGBA{R: 0x66, G: 0xE0, B: 0x5D, A: 0xFF}

// stepPhysics drives the drag integrator for a fixed number of ~60fps frames, so
// convergence tests are deterministic and do not depend on the fyne animation
// driver or wall-clock timing.
func stepPhysics(m *Mascot, frames int) {
	const dt = 1.0 / 60
	for i := 0; i <= frames; i++ {
		m.stepPhysics(dt)
	}
}

// mascotCenter is the stage-relative point at the centre of the mascot's box, used
// to start a grab gesture on the mascot itself.
func mascotCenter(m *Mascot) fyne.Position {
	homeX := (m.stageSize.Width - mascotSize) / 2
	homeY := (m.stageSize.Height - mascotSize) / 2
	return fyne.NewPos(homeX+mascotSize/2+float32(m.posX), homeY+mascotSize/2+float32(m.posY))
}

// TestDragOffMascotIsIgnored verifies a gesture that starts on the empty stage (not
// on the mascot) never grabs it, so clicking blank sidebar space does nothing.
func TestDragOffMascotIsIgnored(t *testing.T) {
	test.NewApp()

	m := New(testStroke)
	m.stageSize = fyne.NewSize(200, 300) // mascot box is [50,150]x[100,200]

	// Start the gesture in the top-left corner, well off the mascot.
	m.drag(fyne.NewPos(5, 5), 60, 60)
	stepPhysics(m, 60)

	if m.dragging {
		t.Fatal("a gesture that began off the mascot must not grab it")
	}
	if m.posX != 0 || m.posY != 0 {
		t.Fatalf("mascot must not move for an off-mascot drag: pos=(%.3f,%.3f)", m.posX, m.posY)
	}
	m.dragEnd()
}

// TestFastFlickGrabsFromPressOrigin verifies the gesture hit-test uses the press
// origin (Position - Dragged), not the already-advanced first drag position, so a
// fast opening flick that begins on the mascot still grabs even when its first
// reported position has jumped off the box.
func TestFastFlickGrabsFromPressOrigin(t *testing.T) {
	test.NewApp()

	m := New(testStroke)
	s, ok := m.Object().(*stage)
	if !ok {
		t.Fatalf("Object() = %T, want *stage", m.Object())
	}
	m.stageSize = fyne.NewSize(200, 300) // mascot box is [50,150]x[100,200]

	// Press at the mascot centre (100,150) then a fast first move of +80,+0 lands the
	// reported Position at (180,150), off the box, but the origin (180-80,150) is on it.
	s.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(180, 150)}, Dragged: fyne.NewDelta(80, 0)})
	if !m.dragging {
		t.Fatal("a fast flick starting on the mascot should grab via the press origin")
	}
	m.dragEnd()

	// Conversely, a gesture whose press origin is off the box must not grab even if
	// the first reported position lands on it: press (20,150) -> Position (100,150).
	m2 := New(testStroke)
	s2 := m2.Object().(*stage)
	m2.stageSize = fyne.NewSize(200, 300)
	s2.Dragged(&fyne.DragEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(100, 150)}, Dragged: fyne.NewDelta(80, 0)})
	if m2.dragging {
		t.Fatal("a gesture whose press origin was off the mascot must not grab")
	}
	m2.dragEnd()
}

// TestRequestDuringDragDoesNotSkipFirstAct verifies that starting a request while
// the ticker is already live for the drag spring re-seeds the act timeline, so the
// first routine act is not skipped by a stale actStart.
func TestRequestDuringDragDoesNotSkipFirstAct(t *testing.T) {
	test.NewApp()

	m := New(testStroke)
	m.stageSize = fyne.NewSize(200, 300)

	// Grab the mascot: the ticker is now live for the spring, but no act plays.
	m.drag(mascotCenter(m), 30, 0)
	if !m.running || m.routinePlaying {
		t.Fatalf("expected drag-only ticker: running=%v routinePlaying=%v", m.running, m.routinePlaying)
	}
	m.actStart = time.Time{} // stale/never-set act start

	m.StartRoutine()

	if !m.routinePlaying {
		t.Fatal("starting a request should mark the routine playing")
	}
	if m.actStart.IsZero() {
		t.Fatal("starting a request over a drag must re-seed actStart so the first act is not skipped")
	}
	if m.actIndex != 0 {
		t.Fatalf("the routine should still begin at its first act, got index %d", m.actIndex)
	}
}

// TestDeformedFramesAreNotCached verifies jelly-deformed frames are rendered fresh
// (not stored in the persistent frame map) while plain frames still memoise, so a
// long interaction cannot grow the cache without bound.
func TestDeformedFramesAreNotCached(t *testing.T) {
	test.NewApp()

	m := New(testStroke)

	before := len(m.frameCache)
	m.applyFrame(restPose(), deform{squashX: 0.2, handR: vec2{4, 3}})
	if len(m.frameCache) != before {
		t.Fatalf("a deformed frame must not be cached: cache %d -> %d", before, len(m.frameCache))
	}

	m.lastName = ""
	m.applyFrame(actSpin().fn(quantizeTime(0.3)), deform{})
	if len(m.frameCache) == before {
		t.Fatal("a plain (undeformed) frame should still be cached")
	}
}

// TestDanglerWhipsThenSettles verifies the appendage dynamics: a target that moves
// then stops leaves the dangler lagging (it whips), and given time it rings down to
// rest so the ticker can stop.
func TestDanglerWhipsThenSettles(t *testing.T) {
	const dt = 1.0 / 60
	var d dangler

	// Seed at the origin, then yank the target sideways over several frames.
	d.step(0, 0, dt)
	for i := 0; i < 8; i++ {
		d.step(float64(i+1)*6, 0, dt) // target accelerates to the right
	}
	// Stop the target dead: the dangler should still be deflected (whipping).
	stopX := 48.0
	d.step(stopX, 0, dt)
	if math.Hypot(d.ox, d.oy) < 0.5 {
		t.Fatalf("dangler should lag after a sudden stop, got offset (%.2f,%.2f)", d.ox, d.oy)
	}

	// Hold the target still: it must ring down to rest.
	for i := 0; i < 600; i++ {
		d.step(stopX, 0, dt)
	}
	if !d.atRest() {
		t.Fatalf("dangler should settle to rest, got offset (%.3f,%.3f) vel (%.3f,%.3f)", d.ox, d.oy, d.vx, d.vy)
	}
}

// appendageSet bundles the five appendage positions for readable assertions.
type appendageSet struct{ bulb, handL, handR, footL, footR vec2 }

func newAppendageSet(bulb, handL, handR, footL, footR vec2) appendageSet {
	return appendageSet{bulb, handL, handR, footL, footR}
}

func baseField(s appendageSet, name string) vec2 {
	switch name {
	case "handL":
		return s.handL
	case "footL":
		return s.footL
	case "footR":
		return s.footR
	case "bulb":
		return s.bulb
	default:
		return s.handR
	}
}

// TestSquishFoldsOnlyTheProtrudingLimbs verifies the squish: pressing a wall folds
// the limbs reaching toward it back toward the body, and leaves the others (and the
// body) in place. Each limb reacts on its own.
func TestSquishFoldsOnlyTheProtrudingLimbs(t *testing.T) {
	rest := restPose()
	base := newAppendageSet(appendages(rest, 0, 0))

	// Press RIGHT: the right hand (reaching to x=59, past the body edge 51) folds
	// left toward the body; nothing else moves.
	right := newAppendageSet(appendages(rest, dragMaxSquish, 0))
	if !(right.handR.x < base.handR.x-1) {
		t.Fatalf("right press should fold the right hand toward the body: %.2f -> %.2f", base.handR.x, right.handR.x)
	}
	if right.handR.x < bodyRightBG-0.01 {
		t.Fatalf("the right hand should not fold past the body edge %.1f, got %.2f", float64(bodyRightBG), right.handR.x)
	}
	for name, got := range map[string]vec2{"handL": right.handL, "footL": right.footL, "footR": right.footR, "bulb": right.bulb} {
		if got != baseField(base, name) {
			t.Fatalf("right press should not move %s: %v -> %v", name, baseField(base, name), got)
		}
	}

	// Press DOWN: both feet (at y=60, past the body bottom 47) fold up; the hands and
	// bulb stay.
	down := newAppendageSet(appendages(rest, 0, dragMaxSquish))
	if !(down.footL.y < base.footL.y-1 && down.footR.y < base.footR.y-1) {
		t.Fatalf("down press should fold the feet up: L %.2f->%.2f R %.2f->%.2f", base.footL.y, down.footL.y, base.footR.y, down.footR.y)
	}
	if down.handL != base.handL || down.handR != base.handR {
		t.Fatal("down press should not move the hands")
	}

	// Press UP: the antenna bulb (at y=5, above the body top 13) folds down.
	up := newAppendageSet(appendages(rest, 0, -dragMaxSquish))
	if !(up.bulb.y > base.bulb.y+1) {
		t.Fatalf("up press should fold the antenna down: %.2f -> %.2f", base.bulb.y, up.bulb.y)
	}
}

// TestFullPressFoldsLimbToBodyEdge verifies a limb pressed at full strength folds
// almost all the way back to the body edge (and never past it).
func TestFullPressFoldsLimbToBodyEdge(t *testing.T) {
	rest := restPose()
	_, _, handR, _, _ := appendages(rest, dragMaxSquish, 0)

	const reach = 8 // neutral right hand sits bodyRightBG+8
	folded := (float64(bodyRightBG) + reach) - handR.x
	if folded < reach*foldStrength-0.01 {
		t.Fatalf("full press should fold the right hand ~%.0f%% of its reach; folded only %.2f of %d",
			foldStrength*100, folded, reach)
	}
	if handR.x < bodyRightBG-0.01 {
		t.Fatalf("the hand must not fold past the body edge %.1f, got %.2f", float64(bodyRightBG), handR.x)
	}
}

// TestDanglerLagChangesRender verifies an appendage lag is plumbed through the frame
// identity and the rendered SVG (so a whip actually draws).
func TestDanglerLagChangesRender(t *testing.T) {
	rest := restPose()

	if frameName(testStroke, rest, deform{}) == frameName(testStroke, rest, deform{bulb: vec2{5, 4}}) {
		t.Fatal("an appendage lag must have a distinct frame name")
	}
	if string(renderPose(testStroke, rest, deform{}).Content()) ==
		string(renderPose(testStroke, rest, deform{handR: vec2{6, 4}}).Content()) {
		t.Fatal("lagging a hand should change the rendered SVG")
	}
}

// TestRubberBandStaysWithinLimit checks the resistance mapping: inside the soft zone
// the offset tracks the pull directly, and no matter how hard the cursor pulls the
// offset never reaches past the wall while the overshoot keeps growing.
func TestRubberBandStaysWithinLimit(t *testing.T) {
	const limit = 50.0
	soft := limit * dragSoftFraction

	// Direct follow inside the soft zone.
	if pos, over := rubberBand(soft*0.5, limit); pos != soft*0.5 || over != 0 {
		t.Fatalf("soft-zone pull should follow directly: pos=%.3f over=%.3f", pos, over)
	}

	cases := []float64{soft + 1, limit, 2 * limit, 100 * limit}
	prev := 0.0
	for _, pull := range cases {
		pos, over := rubberBand(pull, limit)
		if pos > limit {
			t.Fatalf("pull %.1f: pos %.4f pulled past the wall %.1f", pull, pos, limit)
		}
		if pos < prev {
			t.Fatalf("pull %.1f: pos %.4f not monotonically non-decreasing (prev %.4f)", pull, pos, prev)
		}
		if over < 0 {
			t.Fatalf("pull %.1f: overshoot %.4f must be non-negative", pull, over)
		}
		prev = pos
	}

	// Symmetry: pulling the other way mirrors the offset and keeps overshoot >= 0.
	if pos, over := rubberBand(-2*limit, limit); pos > -soft || over < 0 {
		t.Fatalf("negative pull should mirror: pos=%.3f over=%.3f", pos, over)
	}

	// A degenerate (zero-size) stage allows no roam.
	if pos, over := rubberBand(100, 0); pos != 0 || over != 0 {
		t.Fatalf("zero limit should pin at home: pos=%.3f over=%.3f", pos, over)
	}
}

// TestSquishAmountRampsAndCaps checks the overshoot -> compression mapping.
func TestSquishAmountRampsAndCaps(t *testing.T) {
	if got := squishAmount(0); got != 0 {
		t.Fatalf("no overshoot should not squish: %.3f", got)
	}
	half := squishAmount(dragSquishRange / 2)
	if math.Abs(half-dragMaxSquish/2) > 1e-9 {
		t.Fatalf("half-range squish = %.4f, want %.4f", half, dragMaxSquish/2)
	}
	if got := squishAmount(dragSquishRange * 10); got != dragMaxSquish {
		t.Fatalf("far overshoot should cap at max squish: got %.4f want %.4f", got, dragMaxSquish)
	}
}

// TestDragPinsAgainstWallAndSquishes verifies that dragging far past a wall leaves
// the mascot pinned just inside that wall (never through it) and squished against it,
// with the perpendicular axis untouched.
func TestDragPinsAgainstWallAndSquishes(t *testing.T) {
	test.NewApp()

	m := New(testStroke)
	m.stageSize = fyne.NewSize(200, 300)
	limX, _ := m.roamLimits()

	m.drag(mascotCenter(m), 500, 0) // grab the mascot and yank hard right
	stepPhysics(m, 400)             // let the spring and squish converge

	if m.posX <= 0 || m.posX > limX {
		t.Fatalf("mascot should be pinned inside the right wall: posX=%.3f limit=%.3f", m.posX, limX)
	}
	if limX-m.posX > 1 {
		t.Fatalf("mascot should be right up against the wall: posX=%.3f limit=%.3f", m.posX, limX)
	}
	if m.squishX <= 0 {
		t.Fatalf("pulling past the wall should squish toward it: squishX=%.4f", m.squishX)
	}
	if m.squishX > dragMaxSquish+1e-6 {
		t.Fatalf("squish must not exceed the max: squishX=%.4f", m.squishX)
	}
	if math.Abs(m.posY) > 0.01 || m.squishY != 0 {
		t.Fatalf("a purely horizontal pull must not move/squish vertically: posY=%.3f squishY=%.4f", m.posY, m.squishY)
	}

	// The physics must have actually re-placed the image object: home is the stage
	// centre, and a right pin moves it right by the offset.
	homeX := (float64(m.stageSize.Width) - float64(mascotSize)) / 2
	wantX := float32(homeX + m.posX)
	if got := m.image.Position().X; math.Abs(float64(got-wantX)) > 0.5 {
		t.Fatalf("mascot image not placed at its drag offset: gotX=%.3f wantX=%.3f", got, wantX)
	}
}

// TestDragReleaseSpringsHomeAndSettles verifies that releasing a dragged mascot lets
// the spring pull it back home, clears the squish, and reports settled.
func TestDragReleaseSpringsHomeAndSettles(t *testing.T) {
	test.NewApp()

	m := New(testStroke)
	m.stageSize = fyne.NewSize(200, 300)

	m.drag(mascotCenter(m), 120, -80)
	stepPhysics(m, 120)
	if m.physicsSettled() {
		t.Fatal("mascot should not be settled while still being dragged")
	}

	m.dragEnd()
	stepPhysics(m, 600) // give the spring ample time to return home

	if !m.physicsSettled() {
		t.Fatalf("released mascot should settle: pos=(%.3f,%.3f) vel=(%.3f,%.3f) squish=(%.4f,%.4f)",
			m.posX, m.posY, m.velX, m.velY, m.squishX, m.squishY)
	}
	if m.posX != 0 || m.posY != 0 {
		t.Fatalf("settled mascot should snap to home: pos=(%.3f,%.3f)", m.posX, m.posY)
	}
}

// TestDragStartsTickerWithoutRoutine verifies that grabbing the mascot runs the
// ticker for the drag spring without starting an act routine, and that once the
// spring is home the ticker settles and stops.
func TestDragStartsTickerWithoutRoutine(t *testing.T) {
	test.NewApp()

	m := New(testStroke)
	m.stageSize = fyne.NewSize(200, 300)

	m.drag(mascotCenter(m), 40, 0)
	if !m.running {
		t.Fatal("dragging should start the ticker")
	}
	if m.routinePlaying {
		t.Fatal("dragging must not start an act routine")
	}

	// While dragging, a tick must keep the ticker alive.
	m.tick(0)
	if !m.running {
		t.Fatal("ticker should stay live while dragging")
	}

	// Release and bring the spring home, then a tick should settle and stop it.
	m.dragEnd()
	stepPhysics(m, 600)
	m.tick(0)
	if m.running {
		t.Fatal("once released and home, the ticker should stop")
	}
}
