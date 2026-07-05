package mascot

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// TestClickPlaysReactionWhenIdle verifies that tapping the idle mascot starts the
// ticker and queues a one-shot reaction act.
func TestClickPlaysReactionWhenIdle(t *testing.T) {
	test.NewApp()

	m := New(testStroke)
	m.stageSize = fyne.NewSize(200, 300)
	s, ok := m.Object().(*stage)
	if !ok {
		t.Fatalf("Object() = %T, want *stage", m.Object())
	}
	if m.running {
		t.Fatal("mascot should be idle before any click")
	}

	s.Tapped(&fyne.PointEvent{Position: mascotCenter(m)})

	if !m.running {
		t.Fatal("clicking the mascot should start the animation ticker")
	}
	if m.oneShot == nil {
		t.Fatal("clicking the mascot should queue a one-shot reaction act")
	}
	if m.active {
		t.Fatal("a click reaction must not mark the mascot request-active")
	}
}

// TestReactionFinishSettlesWhenIdle verifies that once a click reaction has played
// out with no request active, the mascot settles back to rest and stops the ticker.
func TestReactionFinishSettlesWhenIdle(t *testing.T) {
	test.NewApp()

	m := New(testStroke)
	m.playAct(actSpin())
	if !m.running || m.oneShot == nil {
		t.Fatal("playAct should start the ticker with a one-shot act")
	}

	// Pretend the act started long enough ago that it has finished.
	m.actStart = time.Now().Add(-10 * time.Second)
	m.tick(0)

	if m.oneShot != nil {
		t.Fatal("a finished reaction should clear the one-shot act")
	}
	if m.running {
		t.Fatal("with no request active, a finished reaction should stop the ticker")
	}
}

// TestReactionAvoidsImmediateRepeat checks the no-repeat guard so two consecutive
// random reactions are not identical (when more than one exists).
func TestReactionAvoidsImmediateRepeat(t *testing.T) {
	test.NewApp()

	m := New(testStroke)
	if len(reactionActs()) < 2 {
		t.Skip("no-repeat guard only matters with multiple reactions")
	}
	for i := 0; i < 30; i++ {
		prev := m.lastReaction
		m.playRandomReaction()
		if m.lastReaction == prev && i > 0 {
			t.Fatalf("reaction %d repeated index %d", i, prev)
		}
	}
}

// TestFramesAreBoundedAndRecur verifies that because each act's timeline is
// quantized to fps, the set of distinct rendered frames is finite and identical on
// every loop: sampling the whole routine + reactions repeatedly must not grow the
// frame set, and the set must be modest.
func TestFramesAreBoundedAndRecur(t *testing.T) {
	test.NewApp()

	acts := append(routine(), reactionActs()...)
	seen := map[string]bool{}
	sampleAll := func() {
		for _, a := range acts {
			for f := 0; ; f++ {
				qt := float64(f) / fps
				if qt >= a.dur {
					break
				}
				seen[poseName(testStroke, quantizePose(a.fn(qt)))] = true
			}
		}
	}

	sampleAll()
	first := len(seen)
	t.Logf("distinct mascot frames across routine + reactions: %d", first)
	for i := 0; i < 10; i++ {
		sampleAll()
	}
	if got := len(seen); got != first {
		t.Fatalf("frame set grew across loops: %d -> %d; quantized frames must recur", first, got)
	}
	if first == 0 {
		t.Fatal("expected a non-empty frame set")
	}
	if first > 2000 {
		t.Fatalf("distinct frame set = %d, unexpectedly large (quantization may be broken)", first)
	}
}

// TestFrameCacheReusesResources verifies applyFrame memoizes rendered (undeformed)
// frames, so re-applying the same pose returns the identical resource object (no
// rebuild) and does not grow the cache.
func TestFrameCacheReusesResources(t *testing.T) {
	test.NewApp()

	m := New(testStroke)

	p := actSpin().fn(quantizeTime(0.3))
	m.applyFrame(p, deform{})
	res1 := m.image.Resource
	size1 := len(m.frameCache)

	// Force a re-apply of the same pose (clear the last-name shortcut).
	m.lastName = ""
	m.applyFrame(p, deform{})
	res2 := m.image.Resource

	if res1 != res2 {
		t.Fatal("re-applying the same pose should reuse the cached resource object")
	}
	if len(m.frameCache) != size1 {
		t.Fatalf("cache grew on a repeated pose: %d -> %d", size1, len(m.frameCache))
	}
}
