package gui

import (
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// The agent-to-host data signal: a minimized element on the RESPONSE heading line
// showing a packet travelling to the host and back while a request is in flight.
// It is separate from the sidebar mascot (see internal/gui/mascot) and rides its
// own small animation ticker, started and stopped with the request.
const (
	signalDotCount = 20

	// signalLaneCycle is how long the packet takes to travel to the host and back.
	signalLaneCycle = 1.3

	// Host receive reaction: the shake oscillates at signalHostShakeFreq cycles per
	// lane cycle and ramps in once the packet passes signalHostReceiveStart.
	signalHostReceiveStart float32 = 0.7
	signalHostShakeFreq            = 9.0

	// signalHostSize is the box size of the minimized host node.
	signalHostSize float32 = 26

	// signalTickDriver is the forever-repeating fyne animation cadence; the packet
	// phase is read off the wall clock (signalStart), so the value is otherwise
	// irrelevant.
	signalTickDriver = 1 * time.Second
)

// startWorking begins the request "working" visuals: the mascot performs its
// routine and the data signal animates a packet to the host.
func (p *popupWindow) startWorking() {
	if p.mascot != nil {
		p.mascot.StartRoutine()
	}
	p.startSignal()
}

// stopWorking ends them: the mascot plays its current act out and settles to rest,
// while the signal returns to idle immediately (real data flow is over).
func (p *popupWindow) stopWorking() {
	if p.mascot != nil {
		p.mascot.StopRoutine()
	}
	p.stopSignal()
}

// buildSignalRow builds the data signal that sits on the RESPONSE heading line: the
// section label on the left, the travelling-packet data lane stretched across the
// middle, and the receiving host node on the right. The packet animates while a
// request is in flight (see startSignal); idle the lane is a faint dotted rule.
func (p *popupWindow) buildSignalRow() fyne.CanvasObject {
	palette := currentBrandPalette()
	p.responseHeading = brandSectionLabel(sectionResp)
	p.dataLane = newDataLane(signalDotCount)
	p.host = newHostNode(serverIcon(palette.mutedGreen), serverIcon(palette.accentGreen), signalHostSize)

	p.signalRow = container.NewBorder(
		nil, nil,
		container.NewHBox(vCenter(p.responseHeading), hStrut(12)),
		container.NewHBox(hStrut(10), vCenter(p.host)),
		vCenter(p.dataLane),
	)
	return p.signalRow
}

// startSignal begins the travelling-packet animation for a request. It is
// idempotent: re-arming an already-running signal keeps its phase.
func (p *popupWindow) startSignal() {
	if p.dataLane == nil {
		return
	}
	if p.signalRunning {
		return
	}
	p.signalRunning = true
	p.signalStart = time.Now()
	if p.signalAnim == nil {
		p.signalAnim = fyne.NewAnimation(signalTickDriver, p.tickSignal)
		p.signalAnim.Curve = fyne.AnimationLinear
		p.signalAnim.RepeatCount = fyne.AnimationRepeatForever
	}
	p.signalAnim.Start()
}

// stopSignal ends the animation and returns the lane and host to idle: real data
// flow is over, so unlike the mascot there is nothing to play out.
func (p *popupWindow) stopSignal() {
	p.signalRunning = false
	if p.signalAnim != nil {
		p.signalAnim.Stop()
	}
	if p.dataLane != nil {
		p.dataLane.SetIdle()
	}
	if p.host != nil {
		p.host.SetState(0, 0)
	}
}

// tickSignal advances the packet along the lane and the host's receive reaction off
// the wall clock.
func (p *popupWindow) tickSignal(float32) {
	if !p.signalRunning || p.dataLane == nil {
		return
	}
	lane := math.Mod(time.Since(p.signalStart).Seconds()/signalLaneCycle, 1)
	tri := 1 - float32(math.Abs(2*lane-1))
	p.dataLane.SetProgress(tri)
	if p.host != nil {
		level := (tri - signalHostReceiveStart) / (1 - signalHostReceiveStart)
		shake := float32(math.Sin(lane * 2 * math.Pi * signalHostShakeFreq))
		p.host.SetState(level, shake)
	}
}
