package gui

import (
	"math"
	"math/rand/v2"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

const (
	mascotSize float32 = 100

	mascotDotCount = 20

	// mascotBulbBaseR is the antenna bulb's resting radius (in the body grid).
	mascotBulbBaseR = 2.4

	// mascotTickDriver is only the cadence at which the forever-repeating fyne
	// animation calls back; the routine's timeline is driven by a wall clock so
	// each act can have its own duration, so this value is otherwise irrelevant.
	mascotTickDriver = 1 * time.Second

	// mascotFPS quantizes each act's timeline to a fixed frame grid. The fyne
	// driver ticks faster than this, but sampling poses on a grid means the set of
	// distinct frames per act is finite and recurs every loop, so the rendered
	// resources (and Fyne's name-keyed SVG raster cache) stay bounded. 30 fps is
	// smooth for a small mascot while keeping the frame set small.
	mascotFPS = 30.0

	// mascotLaneCycle is how long the data packet takes to travel to the host and
	// back while a request is in flight.
	mascotLaneCycle = 1.3

	// Host receive reaction: the shake oscillates at hostShakeFreq cycles per lane
	// cycle and ramps in once the packet passes hostReceiveStart.
	hostReceiveStart float32 = 0.7
	hostShakeFreq            = 9.0

	// signalHostSize is the box size of the minimized host node in the response
	// header signal row (the bottom hero panel used a 40px node).
	signalHostSize float32 = 26

	// Act durations (seconds). Acts whose motion is periodic use durations that
	// land the mascot back at rest so the routine chains without popping.
	actDurIdle   = 2.6
	actDurWalk   = 3.2
	actDurJump   = 1.4
	actDurFlip   = 1.3
	actDurWave   = 1.8
	actDurLook   = 2.4
	actDurZap    = 1.7
	actDurDance  = 2.5
	actDurType   = 1.6
	actDurSpin   = 0.9
	actDurWobble = 1.2
)

// mascotPose fully describes one drawable frame. Per-part offsets are in the
// mascot's own 0..64 drawing grid (a negative bob/DY moves a part up); the
// whole-body transform fields (tx, ty, rot, squash) are applied on top in the
// larger canvas so acts can move, rotate, and squash the entire mascot for big
// motions like jumps and flips. renderMascotPose turns a pose into an SVG.
type mascotPose struct {
	// Per-part motion, in the 0..64 body grid.
	bob       float64 // upper-body vertical bounce (negative = up)
	antennaDX float64 // antenna horizontal sway
	bulbR     float64 // antenna bulb radius (pulses)
	zap       float64 // antenna zap bolt length, 0 = no bolt
	armLDY    float64 // left hand vertical offset
	armRDY    float64 // right hand vertical offset
	legLDX    float64 // left foot horizontal offset
	legRDX    float64 // right foot horizontal offset
	cursorOn  bool    // terminal cursor (the >_ underscore) visible

	// Whole-body transform, in canvas units.
	tx     float64 // horizontal translation
	ty     float64 // vertical translation (negative = up)
	rot    float64 // rotation in degrees about the body centre
	squash float64 // vertical scale about the feet (1 = none); width compensates
}

// basePose is the relaxed standing mascot every act starts and ends from, so
// acts chain together seamlessly.
func basePose() mascotPose {
	return mascotPose{bulbR: mascotBulbBaseR, cursorOn: true, squash: 1}
}

// mascotRestPose is the still mascot shown between requests.
func mascotRestPose() mascotPose { return basePose() }

// mascotAct is one self-contained mini-performance: pose returns the mascot pose
// at local time t (seconds) within the act, for t in [0, dur].
type mascotAct struct {
	name string
	dur  float64
	pose func(t float64) mascotPose
}

// --- easing helpers -------------------------------------------------------

func clamp01(x float64) float64 {
	switch {
	case x < 0:
		return 0
	case x > 1:
		return 1
	default:
		return x
	}
}

// ease is smoothstep: 0 at x<=0, 1 at x>=1, smooth in between.
func ease(x float64) float64 {
	x = clamp01(x)
	return x * x * (3 - 2*x)
}

// hump rises from 0 to 1 and back to 0 over u in [0,1] (one smooth bump).
func hump(u float64) float64 { return math.Sin(math.Pi * clamp01(u)) }

func lerp(a, b, x float64) float64 { return a + (b-a)*x }

// quantizeMascotTime snaps an act-local time to the mascotFPS frame grid so the
// same frame index always maps to the same pose (and resource name) across loops.
func quantizeMascotTime(t float64) float64 { return math.Floor(t*mascotFPS) / mascotFPS }

// quantizeMascotPose snaps a pose to the same grid mascotPoseName encodes, so a
// frame's cache name and its rendered SVG are always consistent: one name maps
// to exactly one rendered pose. The grid is far finer than a pixel, so the snap
// is invisible. Keep the step factors here in sync with mascotPoseName.
func quantizeMascotPose(p mascotPose) mascotPose {
	round := func(v, step float64) float64 { return math.Round(v/step) * step }
	const g = 0.1 // matches the *10 fields in mascotPoseName
	p.bob = round(p.bob, g)
	p.antennaDX = round(p.antennaDX, g)
	p.bulbR = round(p.bulbR, g)
	p.armLDY = round(p.armLDY, g)
	p.armRDY = round(p.armRDY, g)
	p.legLDX = round(p.legLDX, g)
	p.legRDX = round(p.legRDX, g)
	p.tx = round(p.tx, g)
	p.ty = round(p.ty, g)
	p.rot = round(p.rot, g)
	p.zap = round(p.zap, 0.01)        // *100 in mascotPoseName
	p.squash = round(p.squash, 0.001) // *1000 in mascotPoseName
	return p
}

// --- the act library ------------------------------------------------------

// actIdle: a calm breather between bigger acts. The mascot breathes (a slow
// squash), the antenna drifts, and the cursor blinks.
func actIdle() mascotAct {
	return mascotAct{"idle", actDurIdle, func(t float64) mascotPose {
		p := basePose()
		// Envelope the breather so idle starts and ends exactly at rest, chaining
		// seamlessly with the acts on either side.
		env := hump(t / actDurIdle)
		p.squash = 1 + 0.03*env*math.Sin(2*math.Pi*t/1.8)
		p.ty = -0.6 * env * (0.5 - 0.5*math.Cos(2*math.Pi*t/1.8))
		p.antennaDX = 0.7 * env * math.Sin(2*math.Pi*t/2.2)
		p.bulbR = mascotBulbBaseR + 0.3*env*math.Sin(2*math.Pi*t/1.1)
		u := t / actDurIdle
		p.cursorOn = u < 0.04 || u > 0.96 || math.Mod(t, 0.9) < 0.6
		return p
	}}
}

// actWalk: strolls to the right and back with a bouncing gait, arms swinging.
func actWalk() mascotAct {
	return mascotAct{"walk", actDurWalk, func(t float64) mascotPose {
		p := basePose()
		gait := 2 * math.Pi * 6 * t / actDurWalk // six strides across the act
		s := math.Sin(gait)
		p.legLDX, p.legRDX = 5.5*s, -5.5*s
		p.armLDY, p.armRDY = 3.0*s, -3.0*s
		p.bob = -2.0 * (0.5 - 0.5*math.Cos(2*gait)) // a bounce on every step
		p.tx = 16 * math.Sin(math.Pi*t/actDurWalk)  // out to the right and back
		return p
	}}
}

// actJump: anticipation crouch, an explosive leap with squash-and-stretch, then
// a landing compression and recovery.
func actJump() mascotAct {
	return mascotAct{"jump", actDurJump, func(t float64) mascotPose {
		p := basePose()
		u := t / actDurJump
		switch {
		case u < 0.22: // crouch
			k := ease(u / 0.22)
			p.squash = lerp(1, 0.78, k)
			p.ty = lerp(0, 4, k)
			p.armLDY, p.armRDY = lerp(0, 5, k), lerp(0, 5, k)
		case u < 0.47: // launch + rise
			k := ease((u - 0.22) / 0.25)
			p.squash = lerp(0.78, 1.12, k)
			p.ty = lerp(4, -20, k)
			p.legLDX, p.legRDX = lerp(0, 2.5, k), lerp(0, -2.5, k)
			p.armLDY, p.armRDY = lerp(5, -5, k), lerp(5, -5, k)
		case u < 0.72: // fall + land
			k := ease((u - 0.47) / 0.25)
			p.squash = lerp(1.12, 0.78, k)
			p.ty = lerp(-20, 0, k)
			p.legLDX, p.legRDX = lerp(2.5, 0, k), lerp(-2.5, 0, k)
			p.armLDY, p.armRDY = lerp(-5, 3, k), lerp(-5, 3, k)
		default: // recover
			k := ease((u - 0.72) / 0.28)
			p.squash = lerp(0.78, 1, k)
			p.armLDY, p.armRDY = lerp(3, 0, k), lerp(3, 0, k)
		}
		return p
	}}
}

// actFlip: a crouch, then a full backflip on an upward arc, then a landing.
func actFlip() mascotAct {
	return mascotAct{"flip", actDurFlip, func(t float64) mascotPose {
		p := basePose()
		u := t / actDurFlip
		switch {
		case u < 0.2: // crouch
			k := ease(u / 0.2)
			p.squash = lerp(1, 0.8, k)
			p.ty = lerp(0, 3, k)
		case u < 0.85: // flip
			a := (u - 0.2) / 0.65
			p.rot = -360 * ease(a) // one full revolution
			p.ty = lerp(3, 0, a) - 18*hump(a)
			p.squash = 1.05
			p.legLDX, p.legRDX = 3, -3 // tuck
		default: // land
			k := ease((u - 0.85) / 0.15)
			p.squash = lerp(0.82, 1, k)
		}
		return p
	}}
}

// actWave: a big hello, raising the right hand overhead and waving with a lean.
func actWave() mascotAct {
	return mascotAct{"wave", actDurWave, func(t float64) mascotPose {
		p := basePose()
		u := t / actDurWave
		env := hump(u)
		p.armRDY = -14*env + 3*env*math.Sin(2*math.Pi*4*u) // up high, wiggling
		p.rot = -4 * env                                   // lean into the wave
		p.bob = -1.5 * (0.5 - 0.5*math.Cos(2*math.Pi*3*u))
		p.antennaDX = 1.2 * math.Sin(2*math.Pi*2*u)
		p.cursorOn = math.Mod(t, 0.7) < 0.45
		return p
	}}
}

// actLook: a curious scan, tilting the whole body side to side with the antenna
// leading the way.
func actLook() mascotAct {
	return mascotAct{"look", actDurLook, func(t float64) mascotPose {
		p := basePose()
		u := t / actDurLook
		p.rot = 11 * math.Sin(2*math.Pi*u)
		// Antenna leads the tilt; enveloped so it starts and ends at rest.
		p.antennaDX = hump(u) * 2.4 * math.Sin(2*math.Pi*u+0.5)
		p.ty = -1.0 * math.Abs(math.Sin(2*math.Pi*u))
		p.cursorOn = math.Mod(t, 0.8) < 0.5
		return p
	}}
}

// actZap: charge the antenna (lean back, bulb swells, crackle), fire a long bolt
// with a forward recoil, then settle.
func actZap() mascotAct {
	return mascotAct{"zap", actDurZap, func(t float64) mascotPose {
		p := basePose()
		u := t / actDurZap
		switch {
		case u < 0.45: // charge
			k := ease(u / 0.45)
			p.rot = -7 * k
			p.ty = 2 * k
			p.bulbR = mascotBulbBaseR + 3.0*k
			p.antennaDX = 1.5 * math.Sin(2*math.Pi*8*u) * k
			p.zap = 0.25 * k * math.Abs(math.Sin(2*math.Pi*12*u)) // building crackle
		case u < 0.62: // fire + recoil
			k := ease((u - 0.45) / 0.17)
			p.rot = lerp(-7, 5, k)
			p.ty = lerp(2, 0, k)
			p.bulbR = lerp(mascotBulbBaseR+3.0, mascotBulbBaseR+0.8, k)
			p.zap = lerp(0.25, 1.5, k) // full blast
			p.squash = 1 - 0.06*hump(k)
		default: // settle
			k := ease((u - 0.62) / 0.38)
			p.rot = lerp(5, 0, k)
			p.bulbR = lerp(mascotBulbBaseR+0.8, mascotBulbBaseR, k)
			p.zap = lerp(1.5, 0, k)
		}
		return p
	}}
}

// actDance: a rhythmic side-to-side groove, swaying and rotating on the beat
// with alternating arm pumps.
func actDance() mascotAct {
	return mascotAct{"dance", actDurDance, func(t float64) mascotPose {
		p := basePose()
		beat := 2 * math.Pi * 2.0 * t // a 2 Hz groove
		p.tx = 9 * math.Sin(beat)
		p.rot = 8 * math.Sin(beat)
		p.bob = -2.0 * math.Abs(math.Sin(beat))
		p.armLDY, p.armRDY = 5*math.Sin(beat), -5*math.Sin(beat)
		p.antennaDX = 2.0 * math.Sin(beat)
		// Bulb pulses on the beat and returns to base at the act's end.
		p.bulbR = mascotBulbBaseR + 0.6*math.Abs(math.Sin(beat))
		p.cursorOn = math.Mod(t, 0.4) < 0.25 // blink to the beat
		return p
	}}
}

// actType: lean in and hammer both hands like a furious typist, cursor flickering.
func actType() mascotAct {
	return mascotAct{"type", actDurType, func(t float64) mascotPose {
		p := basePose()
		lean := hump(t / actDurType)
		p.rot = 3 * lean
		p.ty = 1.5 * lean
		fast := 2 * math.Pi * 9 * t
		p.armLDY = 4 * math.Sin(fast) * lean
		p.armRDY = 4 * math.Sin(fast+math.Pi) * lean
		p.bob = -0.8 * math.Abs(math.Sin(fast)) * lean
		// Cursor flickers while typing but rests lit at the start/end (lean→0).
		p.cursorOn = lean < 0.1 || math.Mod(t, 0.18) < 0.12
		return p
	}}
}

// actSpin: a quick, delighted in-place twirl (a full rotation about the centre)
// with a little lift and squash.
func actSpin() mascotAct {
	return mascotAct{"spin", actDurSpin, func(t float64) mascotPose {
		p := basePose()
		u := t / actDurSpin
		p.rot = 360 * ease(u)
		p.ty = -3 * hump(u)
		p.squash = 1 + 0.06*hump(u)
		p.bulbR = mascotBulbBaseR + 0.6*hump(u)
		return p
	}}
}

// actWobble: knocked to one side and wobbling back upright with a damped
// oscillation, like reacting to a poke.
func actWobble() mascotAct {
	return mascotAct{"wobble", actDurWobble, func(t float64) mascotPose {
		p := basePose()
		u := t / actDurWobble
		// Damped oscillation, forced to exactly zero at the end (1-u) so the
		// reaction lands cleanly at rest. The knocked-over start is intentional.
		decay := math.Exp(-3*u) * (1 - u)
		osc := math.Cos(2 * math.Pi * 3 * u) // a few wobbles
		p.rot = 12 * decay * osc
		p.tx = 4 * decay * osc
		p.armLDY = 3 * decay * osc
		p.armRDY = -3 * decay * osc
		return p
	}}
}

// mascotReactionActs are the animations a click can trigger: the big, expressive
// acts plus the two poke-reaction acts (spin, wobble). playRandomMascotReaction
// chooses one at random.
func mascotReactionActs() []mascotAct {
	return []mascotAct{
		actSpin(),
		actWobble(),
		actJump(),
		actFlip(),
		actDance(),
		actZap(),
		actWave(),
	}
}

// mascotRoutine is the curated performance: bigger acts interleaved with idle
// breathers so it reads as a choreographed show rather than a busy twitch. It
// loops, and the popup remembers its place across requests for variety.
func mascotRoutine() []mascotAct {
	return []mascotAct{
		actWave(), // greet
		actIdle(),
		actWalk(),
		actJump(),
		actIdle(),
		actLook(),
		actZap(),
		actDance(),
		actIdle(),
		actType(),
		actFlip(),
		actIdle(),
	}
}

// buildMascot creates the animated mascot and returns it for the sidebar. The
// mascot lives on its own here and performs its routine (see the act library);
// the agent-to-host data signal is a separate, minimized element in the response
// header (buildSignalRow).
func (p *popupWindow) buildMascot() fyne.CanvasObject {
	p.mascotStroke = currentBrandPalette().accentGreen
	p.robotIdle = renderMascotPose(p.mascotStroke, mascotRestPose())
	p.mascotActs = mascotRoutine()
	p.mascotLastReaction = -1 // no reaction played yet; allow any first pick
	p.mascotFrameCache = make(map[string]fyne.Resource)

	p.mascotImage = canvas.NewImageFromResource(p.robotIdle)
	p.mascotImage.FillMode = canvas.ImageFillContain
	p.mascotImage.SetMinSize(fyne.NewSize(mascotSize, mascotSize))
	mascot := container.NewGridWrap(fyne.NewSize(mascotSize, mascotSize), p.mascotImage)
	// Clicking the mascot triggers a random reaction animation.
	return newTappableBox(mascot, p.playRandomMascotReaction)
}

// buildSignalRow builds the minimized agent-to-host data signal that sits on the
// RESPONSE heading line: the section label on the left, the travelling-packet
// data lane stretched across the middle, and the receiving host node on the
// right. The packet animates while a request is in flight (see tickMascot); idle
// the lane is a faint dotted rule.
func (p *popupWindow) buildSignalRow() fyne.CanvasObject {
	palette := currentBrandPalette()
	p.responseHeading = brandSectionLabel(sectionResp)
	p.dataLane = newDataLane(mascotDotCount)
	p.host = newHostNode(serverIcon(palette.mutedGreen), serverIcon(palette.accentGreen), signalHostSize)

	p.signalRow = container.NewBorder(
		nil, nil,
		container.NewHBox(vCenter(p.responseHeading), hStrut(12)),
		container.NewHBox(hStrut(10), vCenter(p.host)),
		vCenter(p.dataLane),
	)
	return p.signalRow
}

// ensureMascotTicker starts the forever-repeating wall-clock ticker if it is not
// already running. The caller sets the act timeline (mascotActStart) first.
func (p *popupWindow) ensureMascotTicker() {
	if p.mascotRunning {
		return
	}
	p.mascotRunning = true
	if p.mascotAnim == nil {
		p.mascotAnim = fyne.NewAnimation(mascotTickDriver, p.tickMascot)
		p.mascotAnim.Curve = fyne.AnimationLinear
		p.mascotAnim.RepeatCount = fyne.AnimationRepeatForever
	}
	p.mascotAnim.Start()
}

// startMascotAnimation begins (or keeps) the mascot routine while a request is
// in flight. It is idempotent: a request already being performed (including one
// mid click-reaction) just re-arms the active flag so the routine keeps chaining.
func (p *popupWindow) startMascotAnimation() {
	if p.mascotImage == nil {
		return
	}
	// (Re)start the data-lane cycle on every inactive→active transition, even if
	// the ticker is already running for a click reaction — otherwise the lane
	// phase would be read from a stale/zero start time.
	if !p.mascotActive {
		p.mascotLaneStart = time.Now()
	}
	p.mascotActive = true
	if p.mascotRunning {
		return
	}
	p.mascotActStart = time.Now()
	p.ensureMascotTicker()
}

// playMascotAct interjects a one-shot act (a click reaction) immediately,
// starting the ticker if idle. When the act finishes, tickMascot resumes the
// request routine if one is active, or settles back to rest otherwise.
func (p *popupWindow) playMascotAct(act mascotAct) {
	if p.mascotImage == nil {
		return
	}
	p.mascotOneShot = &act
	p.mascotActStart = time.Now()
	p.ensureMascotTicker()
}

// playRandomMascotReaction picks a random reaction act (avoiding an immediate
// repeat) and plays it. It backs the mascot's click handler.
func (p *popupWindow) playRandomMascotReaction() {
	acts := mascotReactionActs()
	if len(acts) == 0 {
		return
	}
	i := rand.IntN(len(acts))
	if len(acts) > 1 && i == p.mascotLastReaction {
		i = (i + 1) % len(acts)
	}
	p.mascotLastReaction = i
	p.playMascotAct(acts[i])
}

// stopMascotAnimation decouples the performance from the request: it never
// hard-cuts an act. It marks the request finished and lets tickMascot play the
// current act out to its natural end before settling to rest. Real data flow is
// over, so the lane and host go idle immediately.
func (p *popupWindow) stopMascotAnimation() {
	p.mascotActive = false
	if p.dataLane != nil {
		p.dataLane.SetIdle()
	}
	if p.host != nil {
		p.host.SetState(0, 0)
	}
	if !p.mascotRunning {
		p.showRestMascot()
	}
}

// showRestMascot displays the still resting pose.
func (p *popupWindow) showRestMascot() {
	if p.mascotImage == nil {
		return
	}
	p.mascotImage.Resource = p.robotIdle
	p.mascotImage.Refresh()
	p.mascotLastName = p.robotIdle.Name()
}

// settleMascotToRest ends the routine: it stops the ticker (safe to call from
// within the tick in fyne 2.7.x) and restores the resting mascot and idle lane.
func (p *popupWindow) settleMascotToRest() {
	p.mascotRunning = false
	if p.mascotAnim != nil {
		p.mascotAnim.Stop()
	}
	p.showRestMascot()
	if p.dataLane != nil {
		p.dataLane.SetIdle()
	}
	if p.host != nil {
		p.host.SetState(0, 0)
	}
}

// tickMascot advances the act routine off the wall clock and, while a request is
// active, drives the data lane and host node on their own steady cycle.
func (p *popupWindow) tickMascot(float32) {
	if !p.mascotRunning || p.mascotImage == nil {
		return
	}
	now := time.Now()

	// A click reaction is a one-shot that takes priority over the request
	// routine; otherwise we play the current routine act.
	act := p.mascotOneShot
	if act == nil {
		if len(p.mascotActs) == 0 {
			p.settleMascotToRest()
			return
		}
		a := p.mascotActs[p.mascotActIndex]
		act = &a
	}

	elapsed := now.Sub(p.mascotActStart).Seconds()
	if elapsed >= act.dur {
		switch {
		case p.mascotOneShot != nil:
			// The click reaction finished.
			p.mascotOneShot = nil
			if !p.mascotActive || len(p.mascotActs) == 0 {
				p.settleMascotToRest()
				return
			}
			// Resume the request routine where it left off.
			p.mascotActStart = now
			a := p.mascotActs[p.mascotActIndex]
			act = &a
			elapsed = 0
		case !p.mascotActive:
			// Request finished and the act has played out: settle and stop.
			p.settleMascotToRest()
			return
		default:
			// Chain to the next act in the routine.
			p.mascotActIndex = (p.mascotActIndex + 1) % len(p.mascotActs)
			p.mascotActStart = now
			a := p.mascotActs[p.mascotActIndex]
			act = &a
			elapsed = 0
		}
	}
	p.applyMascotPose(act.pose(quantizeMascotTime(elapsed)))

	if p.mascotActive {
		lane := math.Mod(now.Sub(p.mascotLaneStart).Seconds()/mascotLaneCycle, 1)
		tri := 1 - float32(math.Abs(2*lane-1))
		if p.dataLane != nil {
			p.dataLane.SetProgress(tri)
		}
		if p.host != nil {
			level := (tri - hostReceiveStart) / (1 - hostReceiveStart)
			shake := float32(math.Sin(lane * 2 * math.Pi * hostShakeFreq))
			p.host.SetState(level, shake)
		}
	}
}

// applyMascotPose renders the pose and swaps it in, skipping the refresh when
// the rounded pose is unchanged so a slow or still moment costs nothing.
func (p *popupWindow) applyMascotPose(pose mascotPose) {
	// Snap to the cache grid first, then both name and render use the snapped
	// pose, so a given name always renders identical SVG bytes.
	pose = quantizeMascotPose(pose)
	name := mascotPoseName(p.mascotStroke, pose)
	if name == p.mascotLastName {
		return
	}
	p.mascotLastName = name
	// Reuse the rendered frame if we have drawn this pose before; the quantized
	// timeline makes frames recur, so this stays bounded and avoids rebuilding
	// the SVG on every loop.
	res := p.mascotFrameCache[name]
	if res == nil {
		res = renderMascotPose(p.mascotStroke, pose)
		if p.mascotFrameCache != nil {
			p.mascotFrameCache[name] = res
		}
	}
	p.mascotImage.Resource = res
	p.mascotImage.Refresh()
}
