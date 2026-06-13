package gui

import (
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
)

const (
	mascotSize float32 = 76

	// Mascot "walking" wiggle and the data-transfer dots, played while a request
	// is in flight.
	mascotFrameCount   = 12
	mascotAntennaSwing = 2.4
	mascotLegSwing     = 3.6
	mascotAnimDuration = 1300 * time.Millisecond
	mascotDotCount     = 20

	// Host receive reaction: when the packet's position (the triangle wave)
	// crosses hostReceiveStart, the host begins reacting; the shake oscillates
	// at hostShakeFreq cycles across the animation loop.
	hostReceiveStart float32 = 0.7
	hostShakeFreq            = 9.0

	tagline1 = "Your terminal."
	tagline2 = "My context."
)

func (p *popupWindow) buildMascotPanel() fyne.CanvasObject {
	palette := currentBrandPalette()
	p.robotIdle = robotMascot(palette.accentGreen)
	p.robotFrames = make([]fyne.Resource, mascotFrameCount)
	for i := range p.robotFrames {
		s := math.Sin(2 * math.Pi * float64(i) / float64(mascotFrameCount))
		// Legs scissor in opposition and the antenna sways with them: a walk.
		p.robotFrames[i] = robotMascotFrame(palette.accentGreen, mascotAntennaSwing*s, mascotLegSwing*s, -mascotLegSwing*s)
	}

	p.mascotImage = canvas.NewImageFromResource(p.robotIdle)
	p.mascotImage.FillMode = canvas.ImageFillContain
	p.mascotImage.SetMinSize(fyne.NewSize(mascotSize, mascotSize))
	mascot := container.NewGridWrap(fyne.NewSize(mascotSize, mascotSize), p.mascotImage)

	t1 := canvas.NewText(tagline1, palette.mutedGreen)
	t1.TextStyle = fyne.TextStyle{Monospace: true}
	t2 := canvas.NewText(tagline2, palette.mutedGreen)
	t2.TextStyle = fyne.TextStyle{Monospace: true}
	tagline := container.NewVBox(layout.NewSpacer(), t1, t2, layout.NewSpacer())

	// The wire between the agent and the host, with a packet that travels along
	// it during a request (see tickMascot); idle it is a faint dotted rule.
	p.dataLane = newDataLane(mascotDotCount)

	// Receiving end: the host/model the agent sends to (not the >_ mark, which
	// is the agent's own identity). It shakes and swells as the packet arrives.
	p.host = newHostNode(serverIcon(palette.mutedGreen), serverIcon(palette.accentGreen))

	row := container.NewBorder(
		nil, nil,
		container.NewHBox(container.NewCenter(mascot), hStrut(16), tagline),
		container.NewHBox(hStrut(10), container.NewCenter(p.host)),
		p.dataLane,
	)
	return borderedBox(row, palette.border)
}

// startMascotAnimation plays the walking wiggle and data-transfer dots while a
// request is in flight. It is idempotent.
func (p *popupWindow) startMascotAnimation() {
	if p.mascotImage == nil {
		return
	}
	if p.mascotAnim == nil {
		p.mascotAnim = fyne.NewAnimation(mascotAnimDuration, p.tickMascot)
		p.mascotAnim.Curve = fyne.AnimationLinear
		p.mascotAnim.RepeatCount = fyne.AnimationRepeatForever
	}
	p.mascotAnim.Start()
}

// stopMascotAnimation halts the animation and restores the resting mascot and
// the faint idle dotted rule.
func (p *popupWindow) stopMascotAnimation() {
	if p.mascotAnim != nil {
		p.mascotAnim.Stop()
	}
	if p.mascotImage != nil {
		p.mascotFrame = 0
		p.mascotImage.Resource = p.robotIdle
		p.mascotImage.Refresh()
	}
	if p.dataLane != nil {
		p.dataLane.SetIdle()
	}
	if p.host != nil {
		p.host.SetState(0, 0)
	}
}

// tickMascot advances the walking mascot frame and sends the packet along the
// data lane for a given animation progress in [0,1).
func (p *popupWindow) tickMascot(progress float32) {
	frame := int(progress*float32(mascotFrameCount)) % mascotFrameCount
	if frame != p.mascotFrame {
		p.mascotFrame = frame
		p.mascotImage.Resource = p.robotFrames[frame]
		p.mascotImage.Refresh()
	}

	// Triangle wave: the packet runs to the host and back, suggesting data sent
	// and received.
	tri := 1 - float32(math.Abs(float64(2*progress-1)))
	if p.dataLane != nil {
		p.dataLane.SetProgress(tri)
	}
	// The host reacts as the packet nears it, peaking on arrival, with a fast
	// jitter for the "shake".
	if p.host != nil {
		level := (tri - hostReceiveStart) / (1 - hostReceiveStart)
		shake := float32(math.Sin(float64(progress) * 2 * math.Pi * hostShakeFreq))
		p.host.SetState(level, shake)
	}
}
