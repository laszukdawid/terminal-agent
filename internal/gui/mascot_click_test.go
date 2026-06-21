package gui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestMascotClickPlaysReactionWhenIdle verifies that tapping the idle mascot
// starts the ticker and queues a one-shot reaction act.
func TestMascotClickPlaysReactionWhenIdle(t *testing.T) {
	test.NewApp()

	p := &popupWindow{}
	obj := p.buildMascot()

	tappable, ok := obj.(*tappableBox)
	if !ok {
		t.Fatalf("buildMascot root = %T, want *tappableBox", obj)
	}
	if p.mascotRunning {
		t.Fatal("mascot should be idle before any click")
	}

	tappable.Tapped(&fyne.PointEvent{})

	if !p.mascotRunning {
		t.Fatal("clicking the mascot should start the animation ticker")
	}
	if p.mascotOneShot == nil {
		t.Fatal("clicking the mascot should queue a one-shot reaction act")
	}
	if p.mascotActive {
		t.Fatal("a click reaction must not mark the mascot request-active")
	}
}

// TestMascotReactionFinishSettlesWhenIdle verifies that once a click reaction
// has played out with no request active, the mascot settles back to rest and
// stops the ticker.
func TestMascotReactionFinishSettlesWhenIdle(t *testing.T) {
	test.NewApp()

	p := &popupWindow{}
	p.buildMascot()

	react := actSpin()
	p.playMascotAct(react)
	if !p.mascotRunning || p.mascotOneShot == nil {
		t.Fatal("playMascotAct should start the ticker with a one-shot act")
	}

	// Pretend the act started long enough ago that it has finished.
	p.mascotActStart = time.Now().Add(-10 * time.Second)
	p.tickMascot(0)

	if p.mascotOneShot != nil {
		t.Fatal("a finished reaction should clear the one-shot act")
	}
	if p.mascotRunning {
		t.Fatal("with no request active, a finished reaction should stop the ticker")
	}
}

// TestMascotReactionAvoidsImmediateRepeat checks the no-repeat guard so two
// consecutive random reactions are not identical (when more than one exists).
func TestMascotReactionAvoidsImmediateRepeat(t *testing.T) {
	test.NewApp()

	p := &popupWindow{}
	p.buildMascot()

	if len(mascotReactionActs()) < 2 {
		t.Skip("no-repeat guard only matters with multiple reactions")
	}
	for i := 0; i < 30; i++ {
		prev := p.mascotLastReaction
		p.playRandomMascotReaction()
		if p.mascotLastReaction == prev && i > 0 {
			t.Fatalf("reaction %d repeated index %d", i, prev)
		}
	}
}
