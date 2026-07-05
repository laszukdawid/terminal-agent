package mascot

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"fyne.io/fyne/v2"
)

// Drawing geometry. The mascot is a soft rounded head/body with an inset "screen"
// face showing the >_ prompt, an antenna with a bulb, two arms with hands, and two
// legs. It is authored in a 0..64 body grid and placed into a larger square canvas
// that leaves headroom for big whole-body motions (jumps, flips, walking) applied
// via a transform group.
const (
	// mascotSize is the drawn cell's side in px; the mascot roams within it.
	mascotSize float32 = 100

	// mascotBulbBaseR is the antenna bulb's resting radius (in the body grid).
	mascotBulbBaseR = 2.4

	mascotAntennaX  = 32.0 // antenna/head centre x in the body grid
	mascotZapStroke = 1.8  // thinner stroke for the antenna zap bolt

	mascotCanvas = 96.0 // square SVG canvas, with room around the body to move
	mascotBodyX  = 16.0 // body-grid origin in the canvas: centres the body...
	mascotBodyY  = 20.0 // ...and leaves headroom above for jumps and flips

	// Body-grid edges of the rounded body rect (x 13..51, y 13..47). A limb folds
	// back toward the body once it protrudes past the edge facing a pressed wall
	// (see foldLimb), so at full press it ends up flat against that edge.
	bodyLeftBG   = 13.0
	bodyRightBG  = 51.0
	bodyTopBG    = 13.0
	bodyBottomBG = 47.0

	// Visible figure bounds within the pose canvas at rest: the left/right hands,
	// the antenna bulb top, and the feet. Everything outside this is transparent
	// headroom the acts use for jumps, flips, and walking. Converted to px, they
	// size the boundary pen so it hugs the drawn robot rather than the mostly-empty
	// cell (see drag.go).
	mascotVisLeftC    = mascotBodyX + 5                   // left hand x
	mascotVisRightC   = mascotBodyX + 59                  // right hand x
	mascotVisTopC     = mascotBodyY + 5 - mascotBulbBaseR // antenna bulb top
	mascotVisBottomC  = mascotBodyY + 60                  // feet y
	mascotVisCenterXC = (mascotVisLeftC + mascotVisRightC) / 2
	mascotVisCenterYC = (mascotVisTopC + mascotVisBottomC) / 2
)

// vec2 is a 2D point/offset in the body grid.
type vec2 struct{ x, y float64 }

// deform is the physical, per-frame deformation layered on top of a pose, produced
// by the dynamics simulation (see drag.go): the body squash (signed compression
// per axis, 0 = none) and the inertial lag of each appendage's free end (body-grid
// units). It is kept separate from the pose so the acts stay purely authored
// keyframes while the jelly physics is one shared system.
type deform struct {
	squashX, squashY float64
	bulb             vec2 // antenna bulb lag
	handL, handR     vec2 // hand lag
	footL, footR     vec2 // foot lag
}

func (d deform) isZero() bool {
	return d == deform{}
}

// foldAmount converts a wall's squish pressure to the fraction of a limb's
// protrusion that is folded back, ramping to foldStrength at full press.
func foldAmount(press float64) float64 {
	if dragMaxSquish <= 0 {
		return 0
	}
	return foldStrength * math.Min(1, math.Abs(press)/dragMaxSquish)
}

// foldLimb pushes a limb free-end back toward the body when it protrudes past the
// body edge facing a pressed wall. This is the squish: the wall can't be crossed,
// so a limb pressed against it folds in toward the body by a fraction of however
// far it stuck out. Only limbs actually reaching toward the wall move (protrusion
// > 0), so each limb reacts on its own; a limb on the far side is untouched.
func foldLimb(p vec2, pressX, pressY, by float64) vec2 {
	if pressX != 0 {
		n := math.Copysign(1, pressX)
		edge := bodyRightBG
		if pressX < 0 {
			edge = bodyLeftBG
		}
		if protr := (p.x - edge) * n; protr > 0 {
			p.x -= n * protr * foldAmount(pressX)
		}
	}
	if pressY != 0 {
		n := math.Copysign(1, pressY)
		edge := bodyBottomBG + by
		if pressY < 0 {
			edge = bodyTopBG + by
		}
		if protr := (p.y - edge) * n; protr > 0 {
			p.y -= n * protr * foldAmount(pressY)
		}
	}
	return p
}

// appendages returns the five appendage free-end positions (bulb, hands, feet) in
// body-grid coordinates for a pose under the current wall pressure, but before the
// inertial lag is added: the articulated limb position, folded back toward the body
// against any pressed wall. renderPose adds the lag; the dynamics use these as the
// spring targets, so both agree on where a limb "should" be and the danglers spring
// toward the folded position with a little whip.
func appendages(p pose, pressX, pressY float64) (bulb, handL, handR, footL, footR vec2) {
	by := p.bob
	bulb = foldLimb(vec2{mascotAntennaX + p.antennaDX, 5 + by}, pressX, pressY, by)
	handL = foldLimb(vec2{bodyLeftBG - 8, 30 + by + p.armLDY}, pressX, pressY, by)
	handR = foldLimb(vec2{bodyRightBG + 8, 30 + by + p.armRDY}, pressX, pressY, by)
	footL = foldLimb(vec2{25 + p.legLDX, 60}, pressX, pressY, by)
	footR = foldLimb(vec2{39 + p.legRDX, 60}, pressX, pressY, by)
	return bulb, handL, handR, footL, footR
}

// renderPose renders one frame of the line-art mascot from a pose plus its physical
// deformation. The body stays rigid; the antenna, arms, and legs are the soft
// parts: each is folded back toward the body when pressed against a wall (the
// squish) and carries its inertial lag (the jelly). The pose's whole-body transform
// (act squash about the feet, rotation, then translation) is applied on top for the
// bigger acts.
func renderPose(stroke color.Color, p pose, d deform) fyne.Resource {
	hex := colorHex(stroke)
	by := p.bob // upper-body vertical offset (negative = up)

	bulb, handL, handR, footL, footR := appendages(p, d.squashX, d.squashY)
	bulb = vec2{bulb.x + d.bulb.x, bulb.y + d.bulb.y}
	handL = vec2{handL.x + d.handL.x, handL.y + d.handL.y}
	handR = vec2{handR.x + d.handR.x, handR.y + d.handR.y}
	footL = vec2{footL.x + d.footL.x, footL.y + d.footL.y}
	footR = vec2{footR.x + d.footR.x, footR.y + d.footR.y}

	// The antenna stalk rises from the head top and meets the bulb at its edge
	// along the base->bulb direction, so it stays connected as the bulb folds/whips.
	base := vec2{mascotAntennaX, 13 + by}
	sdx, sdy := bulb.x-base.x, bulb.y-base.y
	sd := math.Hypot(sdx, sdy)
	if sd < 1e-6 {
		sd = 1
	}
	stalkX, stalkY := bulb.x-sdx/sd*p.bulbR, bulb.y-sdy/sd*p.bulbR

	var b strings.Builder
	// Body first, so the limbs and antenna draw on top of it.
	fmt.Fprintf(&b, `<rect x="13" y="%.2f" width="38" height="34" rx="8"/>`, 13+by)
	fmt.Fprintf(&b, `<rect x="19" y="%.2f" width="26" height="18" rx="3"/>`, 20+by)
	fmt.Fprintf(&b, `<polyline points="24,%.2f 28,%.2f 24,%.2f"/>`, 26+by, 29+by, 32+by)
	if p.cursorOn {
		fmt.Fprintf(&b, `<line x1="31" y1="%.2f" x2="39" y2="%.2f"/>`, 33+by, 33+by)
	}

	// Antenna: bulb (pulses) on a stalk rising from the head top.
	fmt.Fprintf(&b, `<circle cx="%.2f" cy="%.2f" r="%.2f"/>`, bulb.x, bulb.y, p.bulbR)
	fmt.Fprintf(&b, `<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`, base.x, base.y, stalkX, stalkY)
	// Antenna zap: a lightning bolt fired sideways from the bulb toward the host on
	// the right. Its length scales with pose.zap (a full blast reaches ~1.5).
	if p.zap > 0 {
		z := p.zap
		cy := bulb.y
		sxz := bulb.x + p.bulbR // start at the bulb's right edge
		fmt.Fprintf(&b,
			`<polyline stroke-width="%.1f" points="%.2f,%.2f %.2f,%.2f %.2f,%.2f %.2f,%.2f"/>`,
			mascotZapStroke,
			sxz, cy,
			sxz+4*z, cy-2.5*z,
			sxz+7*z, cy+1.5*z,
			sxz+11*z, cy-2*z,
		)
	}

	// Arms: shoulder fixed at the body edge, hand folded/whipped to its free end.
	fmt.Fprintf(&b, `<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`, bodyLeftBG, 30+by, handL.x, handL.y)
	fmt.Fprintf(&b, `<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`, bodyRightBG, 30+by, handR.x, handR.y)
	// Legs: hip fixed at the body, foot folded/whipped to its free end.
	fmt.Fprintf(&b, `<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`, 25.0, 47+by, footL.x, footL.y)
	fmt.Fprintf(&b, `<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`, 39.0, 47+by, footR.x, footR.y)

	// Whole-body transform, applied right-to-left to the body grid: place it in the
	// canvas, squash about the feet, rotate about the centre, then translate.
	sq := p.squash
	if sq == 0 {
		sq = 1
	}
	feetX, feetY := mascotBodyX+32, mascotBodyY+60
	cx, cy := mascotBodyX+32, mascotBodyY+30
	transform := fmt.Sprintf(
		"translate(%.2f %.2f) rotate(%.2f %.2f %.2f) translate(%.2f %.2f) scale(%.4f %.4f) translate(%.2f %.2f) translate(%.2f %.2f)",
		p.tx, p.ty, p.rot, cx, cy, feetX, feetY, 1/sq, sq, -feetX, -feetY, mascotBodyX, mascotBodyY,
	)

	return fyne.NewStaticResource(frameName(stroke, p, d), []byte(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" fill="none" stroke="%s" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><g transform="%s">%s</g></svg>`,
		mascotCanvas, mascotCanvas, mascotCanvas, mascotCanvas, hex, transform, b.String(),
	)))
}

// poseName is the identity of a rendered pose. It varies with every pose field that
// changes the drawing; frameName layers the deformation on top.
func poseName(stroke color.Color, p pose) string {
	return fmt.Sprintf("robot-%s-%.0f-%.0f-%.0f-%.0f-%.0f-%.0f-%.0f-%.0f-%t-%.0f-%.0f-%.0f-%.0f",
		colorHex(stroke), p.bob*10, p.antennaDX*10, p.bulbR*10, p.zap*100,
		p.armLDY*10, p.armRDY*10, p.legLDX*10, p.legRDX*10, p.cursorOn,
		p.tx*10, p.ty*10, p.rot*10, p.squash*1000)
}

// frameName is the identity (and raster cache key) of a fully deformed frame: the
// pose plus the body squash and appendage lag. applyFrame compares it to skip the
// SVG build when nothing changed. The deform terms are quantized so
// continuously-varying jelly frames still map to a stable name.
func frameName(stroke color.Color, p pose, d deform) string {
	if d.isZero() {
		return poseName(stroke, p) + ".svg"
	}
	q := func(v float64) float64 { return math.Round(v * 5) } // 0.2-unit grid
	return fmt.Sprintf("%s-%.0f-%.0f-d%.0f,%.0f-%.0f,%.0f-%.0f,%.0f-%.0f,%.0f-%.0f,%.0f.svg",
		poseName(stroke, p), math.Round(d.squashX*50), math.Round(d.squashY*50),
		q(d.bulb.x), q(d.bulb.y), q(d.handL.x), q(d.handL.y), q(d.handR.x), q(d.handR.y),
		q(d.footL.x), q(d.footL.y), q(d.footR.x), q(d.footR.y))
}

func colorHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}
