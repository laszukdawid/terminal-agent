package gui

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

// TestMascotFramesAreBoundedAndRecur verifies the SF3 fix: because each act's
// timeline is quantized to mascotFPS, the set of distinct rendered frames is
// finite and identical on every loop. Sampling the whole routine + reactions
// repeatedly must not grow the frame set, and the set must be modest.
func TestMascotFramesAreBoundedAndRecur(t *testing.T) {
	test.NewApp()

	p := &popupWindow{}
	p.buildMascot()
	stroke := p.mascotStroke

	acts := append(mascotRoutine(), mascotReactionActs()...)
	seen := map[string]bool{}
	sampleAll := func() {
		for _, act := range acts {
			for f := 0; ; f++ {
				qt := float64(f) / mascotFPS
				if qt >= act.dur {
					break
				}
				seen[mascotPoseName(stroke, quantizeMascotPose(act.pose(qt)))] = true
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

	// Sanity bound: catch a regression that accidentally re-introduces unbounded
	// (per-tick) frame generation. The real working set is a few hundred frames.
	if first == 0 {
		t.Fatal("expected a non-empty frame set")
	}
	if first > 2000 {
		t.Fatalf("distinct frame set = %d, unexpectedly large (quantization may be broken)", first)
	}
}

// TestMascotFrameCacheReusesResources verifies applyMascotPose memoizes rendered
// frames, so re-applying the same pose returns the identical resource object
// (no rebuild) and does not grow the cache.
func TestMascotFrameCacheReusesResources(t *testing.T) {
	test.NewApp()

	p := &popupWindow{}
	p.buildMascot()

	pose := actSpin().pose(quantizeMascotTime(0.3))
	p.applyMascotPose(pose)
	res1 := p.mascotImage.Resource
	size1 := len(p.mascotFrameCache)

	// Force a re-apply of the same pose (clear the last-name shortcut).
	p.mascotLastName = ""
	p.applyMascotPose(pose)
	res2 := p.mascotImage.Resource

	if res1 != res2 {
		t.Fatal("re-applying the same pose should reuse the cached resource object")
	}
	if len(p.mascotFrameCache) != size1 {
		t.Fatalf("cache grew on a repeated pose: %d -> %d", size1, len(p.mascotFrameCache))
	}
}
