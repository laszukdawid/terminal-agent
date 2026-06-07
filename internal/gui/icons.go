package gui

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
)

// Feather-style line-icon path data (24x24 viewBox). These back the sidebar
// navigation and the listen control so the icons read as the simple, line-based
// terminal glyphs the brand calls for rather than filled SaaS pictograms.
const (
	iconPathChat     = `<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>`
	iconPathHistory  = `<path d="M3 3v5h5"/><path d="M3.05 13A9 9 0 1 0 6 5.3L3 8"/><path d="M12 7v5l3 3"/>`
	iconPathEnv      = `<polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/>`
	iconPathTools    = `<path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/>`
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

// robotMascotStatic is the fixed part of our line-art terminal agent mascot:
// a soft rounded head with side connectors and an inset "screen" face showing
// the >_ prompt. The terminal-faced design ties the mascot to the product
// identity while staying clean and contemporary. The antenna and legs are
// drawn separately so they can be offset per animation frame.
const robotMascotStatic = `<rect x="13" y="13" width="38" height="34" rx="8"/>` +
	`<line x1="13" y1="30" x2="4" y2="30"/><line x1="51" y1="30" x2="60" y2="30"/>` +
	`<rect x="19" y="20" width="26" height="18" rx="3"/>` +
	`<polyline points="24,26 28,29 24,32"/>` +
	`<line x1="31" y1="33" x2="39" y2="33"/>`

// robotMascotFrame renders the mascot with the antenna nudged by antennaDX and
// the two legs nudged by legLDX/legRDX (in the 64x64 grid). Zero offsets give
// the resting pose; swinging the legs in opposition and the antenna with them
// produces the walking wiggle used while a request is in flight.
func robotMascotFrame(stroke color.Color, antennaDX, legLDX, legRDX float64) fyne.Resource {
	hex := colorHex(stroke)
	paths := fmt.Sprintf(
		`<circle cx="%.2f" cy="5" r="2.4"/><line x1="32" y1="13" x2="%.2f" y2="7.4"/>`+
			robotMascotStatic+
			`<line x1="25" y1="47" x2="%.2f" y2="60"/><line x1="39" y1="47" x2="%.2f" y2="60"/>`,
		32+antennaDX, 32+antennaDX, 25+legLDX, 39+legRDX,
	)
	name := fmt.Sprintf("robot-%s-%.0f-%.0f-%.0f.svg", hex, antennaDX*10, legLDX*10, legRDX*10)
	return fyne.NewStaticResource(name, []byte(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" viewBox="0 0 64 64" fill="none" stroke="%s" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">%s</svg>`,
		hex, paths,
	)))
}

// robotMascot renders the resting mascot in the given color.
func robotMascot(stroke color.Color) fyne.Resource {
	return robotMascotFrame(stroke, 0, 0, 0)
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
