package gui

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
)

// Feather-style line-icon path data (24x24 viewBox). These back the sidebar
// navigation and the listen control so the icons read as the simple, line-based
// terminal glyphs the brand calls for rather than filled SaaS pictograms.
const (
	iconPathChat = `<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>`
	// iconPathTask is a lightning "zap" glyph: the Task tab runs agentic,
	// tool-executing work, so it reads as action rather than the chat bubble.
	iconPathTask    = `<polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/>`
	iconPathHistory = `<path d="M3 3v5h5"/><path d="M3.05 13A9 9 0 1 0 6 5.3L3 8"/><path d="M12 7v5l3 3"/>`
	// iconPathRoutine is a clock glyph: scheduled, recurring runs.
	iconPathRoutine  = `<circle cx="12" cy="12" r="9"/><polyline points="12 7 12 12 15 14"/>`
	iconPathEnv      = `<polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/>`
	iconPathTools    = `<path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/>`
	iconPathTest     = `<path d="M10 2v6.5L4.7 18a3 3 0 0 0 2.6 4h9.4a3 3 0 0 0 2.6-4L14 8.5V2"/><path d="M8 2h8"/><path d="M7.5 14h9"/>`
	iconPathSettings = `<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>`
	iconPathMic      = `<rect x="9" y="2" width="6" height="11" rx="3"/><path d="M5 10a7 7 0 0 0 14 0"/><line x1="12" y1="17" x2="12" y2="21"/><line x1="8" y1="21" x2="16" y2="21"/>`
	iconPathCopy     = `<rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>`
	iconPathExport   = `<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/>`
	// iconPathServer is the host/model endpoint the agent talks to: a stacked
	// server with status lights. It anchors the receiving end of the data lane.
	iconPathServer = `<rect x="2" y="3" width="20" height="8" rx="2"/><rect x="2" y="13" width="20" height="8" rx="2"/><line x1="6" y1="7" x2="6.01" y2="7"/><line x1="6" y1="17" x2="6.01" y2="17"/>`
)

// lineIcon renders an SVG line icon stroked in the given color. The stroke
// color is baked into the resource, so callers request the green and muted
// variants explicitly (e.g. for active vs. inactive navigation rows).
func lineIcon(name, paths string, stroke color.Color) fyne.Resource {
	return fyne.NewStaticResource(name+"-"+colorHex(stroke)+".svg", []byte(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="%s" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">%s</svg>`,
		colorHex(stroke), paths,
	)))
}

// Mascot drawing geometry. The mascot is a soft rounded head/body with an inset
// "screen" face showing the >_ prompt, an antenna with a bulb, two arms with
// hands, and two legs. It is authored in a 0..64 body grid and placed into a
// larger square canvas that leaves headroom for big whole-body motions (jumps,
// flips, walking) applied via a transform group. The terminal-faced design ties
// the mascot to the product identity while staying clean and contemporary.
const (
	mascotAntennaX  = 32.0 // antenna/head centre x in the body grid
	mascotZapStroke = 1.8  // thinner stroke for the antenna zap bolt

	mascotCanvas = 96.0 // square SVG canvas, with room around the body to move
	mascotBodyX  = 16.0 // body-grid origin in the canvas: centres the body...
	mascotBodyY  = 20.0 // ...and leaves headroom above for jumps and flips
)

// renderMascotPose renders one frame of the line-art mascot from a pose. The
// body bob shifts the upper body (head, face, arms, antenna) vertically while
// the feet stay planted, so the legs compress and extend like a real gait; the
// pose's whole-body transform (squash about the feet, rotation about the centre,
// then translation) is applied on top for the bigger acts. See the act library
// in mascot_animation.go for how poses are animated.
func renderMascotPose(stroke color.Color, pose mascotPose) fyne.Resource {
	hex := colorHex(stroke)
	by := pose.bob                        // upper-body vertical offset (negative = up)
	bx := mascotAntennaX + pose.antennaDX // antenna bulb centre x

	var b strings.Builder
	// Antenna: bulb (pulses) on a stalk rising from the head top.
	fmt.Fprintf(&b, `<circle cx="%.2f" cy="%.2f" r="%.2f"/>`, bx, 5+by, pose.bulbR)
	fmt.Fprintf(&b, `<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`, mascotAntennaX, 13+by, bx, 7.4+by)
	// Antenna zap: a lightning bolt fired sideways from the bulb toward the host
	// on the right. Its length scales with pose.zap (a full blast reaches ~1.5).
	if pose.zap > 0 {
		z := pose.zap
		cy := 5 + by
		sx := bx + pose.bulbR // start at the bulb's right edge
		fmt.Fprintf(&b,
			`<polyline stroke-width="%.1f" points="%.2f,%.2f %.2f,%.2f %.2f,%.2f %.2f,%.2f"/>`,
			mascotZapStroke,
			sx, cy,
			sx+4*z, cy-2.5*z,
			sx+7*z, cy+1.5*z,
			sx+11*z, cy-2*z,
		)
	}
	// Head / body.
	fmt.Fprintf(&b, `<rect x="13" y="%.2f" width="38" height="34" rx="8"/>`, 13+by)
	// Arms: shoulder fixed at the body edge, hand swings/waves on the y axis.
	fmt.Fprintf(&b, `<line x1="13" y1="%.2f" x2="5" y2="%.2f"/>`, 30+by, 30+by+pose.armLDY)
	fmt.Fprintf(&b, `<line x1="51" y1="%.2f" x2="59" y2="%.2f"/>`, 30+by, 30+by+pose.armRDY)
	// Inset terminal "screen" face with the >_ prompt; the cursor blinks.
	fmt.Fprintf(&b, `<rect x="19" y="%.2f" width="26" height="18" rx="3"/>`, 20+by)
	fmt.Fprintf(&b, `<polyline points="24,%.2f 28,%.2f 24,%.2f"/>`, 26+by, 29+by, 32+by)
	if pose.cursorOn {
		fmt.Fprintf(&b, `<line x1="31" y1="%.2f" x2="39" y2="%.2f"/>`, 33+by, 33+by)
	}
	// Legs: the hip rides with the body bob; the feet stay on the ground.
	fmt.Fprintf(&b, `<line x1="25" y1="%.2f" x2="%.2f" y2="60"/>`, 47+by, 25+pose.legLDX)
	fmt.Fprintf(&b, `<line x1="39" y1="%.2f" x2="%.2f" y2="60"/>`, 47+by, 39+pose.legRDX)

	// Whole-body transform, applied right-to-left to the body grid: place it in
	// the canvas, squash about the feet, rotate about the centre, then translate.
	sq := pose.squash
	if sq == 0 {
		sq = 1
	}
	feetX, feetY := mascotBodyX+32, mascotBodyY+60
	cx, cy := mascotBodyX+32, mascotBodyY+30
	transform := fmt.Sprintf(
		"translate(%.2f %.2f) rotate(%.2f %.2f %.2f) translate(%.2f %.2f) scale(%.4f %.4f) translate(%.2f %.2f) translate(%.2f %.2f)",
		pose.tx, pose.ty, pose.rot, cx, cy, feetX, feetY, 1/sq, sq, -feetX, -feetY, mascotBodyX, mascotBodyY,
	)

	return fyne.NewStaticResource(mascotPoseName(stroke, pose), []byte(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" fill="none" stroke="%s" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><g transform="%s">%s</g></svg>`,
		mascotCanvas, mascotCanvas, mascotCanvas, mascotCanvas, hex, transform, b.String(),
	)))
}

// mascotPoseName is the identity (and raster cache key) of a rendered pose. It
// varies with every field that changes the drawing, so applyMascotPose can
// compare it and skip both the SVG build and the resource allocation when the
// rounded pose is unchanged.
func mascotPoseName(stroke color.Color, pose mascotPose) string {
	return fmt.Sprintf("robot-%s-%.0f-%.0f-%.0f-%.0f-%.0f-%.0f-%.0f-%.0f-%t-%.0f-%.0f-%.0f-%.0f.svg",
		colorHex(stroke), pose.bob*10, pose.antennaDX*10, pose.bulbR*10, pose.zap*100,
		pose.armLDY*10, pose.armRDY*10, pose.legLDX*10, pose.legRDX*10, pose.cursorOn,
		pose.tx*10, pose.ty*10, pose.rot*10, pose.squash*1000)
}

// robotMascot renders the resting mascot in the given color.
func robotMascot(stroke color.Color) fyne.Resource {
	return renderMascotPose(stroke, mascotRestPose())
}

// serverIcon renders the host/model endpoint with a slightly bolder stroke than
// the standard line icons, so it reads as a solid receiving node.
func serverIcon(stroke color.Color) fyne.Resource {
	hex := colorHex(stroke)
	return fyne.NewStaticResource("server-"+hex+".svg", []byte(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="%s" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round">%s</svg>`,
		hex, iconPathServer,
	)))
}

func colorHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}
