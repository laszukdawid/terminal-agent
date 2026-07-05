package mascot

import "math"

const (
	// fps quantizes each act's timeline to a fixed frame grid. The fyne driver
	// ticks faster than this, but sampling poses on a grid means the set of distinct
	// frames per act is finite and recurs every loop, so the rendered resources (and
	// Fyne's name-keyed SVG raster cache) stay bounded. 30 fps is smooth for a small
	// mascot while keeping the frame set small.
	fps = 30.0

	// Act durations (seconds). Acts whose motion is periodic use durations that land
	// the mascot back at rest so the routine chains without popping.
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

// pose fully describes one drawable frame. Per-part offsets are in the mascot's own
// 0..64 drawing grid (a negative bob/DY moves a part up); the whole-body transform
// fields (tx, ty, rot, squash) are applied on top in the larger canvas so acts can
// move, rotate, and squash the entire mascot for big motions like jumps and flips.
// renderPose turns a pose into an SVG.
type pose struct {
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

// basePose is the relaxed standing mascot every act starts and ends from, so acts
// chain together seamlessly.
func basePose() pose {
	return pose{bulbR: mascotBulbBaseR, cursorOn: true, squash: 1}
}

// restPose is the still mascot shown between requests.
func restPose() pose { return basePose() }

// act is one self-contained mini-performance: pose returns the mascot pose at local
// time t (seconds) within the act, for t in [0, dur].
type act struct {
	name string
	dur  float64
	fn   func(t float64) pose
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

// quantizeTime snaps an act-local time to the fps frame grid so the same frame
// index always maps to the same pose (and resource name) across loops.
func quantizeTime(t float64) float64 { return math.Floor(t*fps) / fps }

// quantizePose snaps a pose to the same grid poseName encodes, so a frame's cache
// name and its rendered SVG are always consistent: one name maps to exactly one
// rendered pose. The grid is far finer than a pixel, so the snap is invisible. Keep
// the step factors here in sync with poseName.
func quantizePose(p pose) pose {
	round := func(v, step float64) float64 { return math.Round(v/step) * step }
	const g = 0.1 // matches the *10 fields in poseName
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
	p.zap = round(p.zap, 0.01)        // *100 in poseName
	p.squash = round(p.squash, 0.001) // *1000 in poseName
	return p
}

// --- the act library ------------------------------------------------------

// actIdle: a calm breather between bigger acts. The mascot breathes (a slow
// squash), the antenna drifts, and the cursor blinks.
func actIdle() act {
	return act{"idle", actDurIdle, func(t float64) pose {
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
func actWalk() act {
	return act{"walk", actDurWalk, func(t float64) pose {
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

// actJump: anticipation crouch, an explosive leap with squash-and-stretch, then a
// landing compression and recovery.
func actJump() act {
	return act{"jump", actDurJump, func(t float64) pose {
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
func actFlip() act {
	return act{"flip", actDurFlip, func(t float64) pose {
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
func actWave() act {
	return act{"wave", actDurWave, func(t float64) pose {
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
func actLook() act {
	return act{"look", actDurLook, func(t float64) pose {
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
func actZap() act {
	return act{"zap", actDurZap, func(t float64) pose {
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

// actDance: a rhythmic side-to-side groove, swaying and rotating on the beat with
// alternating arm pumps.
func actDance() act {
	return act{"dance", actDurDance, func(t float64) pose {
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
func actType() act {
	return act{"type", actDurType, func(t float64) pose {
		p := basePose()
		lean := hump(t / actDurType)
		p.rot = 3 * lean
		p.ty = 1.5 * lean
		fast := 2 * math.Pi * 9 * t
		p.armLDY = 4 * math.Sin(fast) * lean
		p.armRDY = 4 * math.Sin(fast+math.Pi) * lean
		p.bob = -0.8 * math.Abs(math.Sin(fast)) * lean
		// Cursor flickers while typing but rests lit at the start/end (lean->0).
		p.cursorOn = lean < 0.1 || math.Mod(t, 0.18) < 0.12
		return p
	}}
}

// actSpin: a quick, delighted in-place twirl (a full rotation about the centre)
// with a little lift and squash.
func actSpin() act {
	return act{"spin", actDurSpin, func(t float64) pose {
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
func actWobble() act {
	return act{"wobble", actDurWobble, func(t float64) pose {
		p := basePose()
		u := t / actDurWobble
		// Damped oscillation, forced to exactly zero at the end (1-u) so the reaction
		// lands cleanly at rest. The knocked-over start is intentional.
		decay := math.Exp(-3*u) * (1 - u)
		osc := math.Cos(2 * math.Pi * 3 * u) // a few wobbles
		p.rot = 12 * decay * osc
		p.tx = 4 * decay * osc
		p.armLDY = 3 * decay * osc
		p.armRDY = -3 * decay * osc
		return p
	}}
}

// reactionActs are the animations a click can trigger: the big, expressive acts
// plus the two poke-reaction acts (spin, wobble). playRandomReaction chooses one.
func reactionActs() []act {
	return []act{
		actSpin(),
		actWobble(),
		actJump(),
		actFlip(),
		actDance(),
		actZap(),
		actWave(),
	}
}

// routine is the curated performance: bigger acts interleaved with idle breathers
// so it reads as a choreographed show rather than a busy twitch. It loops, and the
// mascot remembers its place across requests for variety.
func routine() []act {
	return []act{
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
