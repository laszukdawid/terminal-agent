package gui

import (
	"strings"
	"testing"
)

func TestResponseMarker(t *testing.T) {
	// Unchanged prompt: no marker at all, just the response.
	if got := responseMarker("Same?", false, true); got != "" {
		t.Errorf("unchanged prompt should have no marker, got %q", got)
	}

	// Changed prompt, first response (no previous output): label, no separator.
	first := responseMarker("What is X?", true, false)
	if !strings.Contains(first, "Response to: What is X?") {
		t.Errorf("marker missing prompt: %q", first)
	}
	if strings.Contains(first, "---") {
		t.Errorf("first response should not have a separator: %q", first)
	}

	// Changed prompt replacing a previous response: separator is prefixed.
	next := responseMarker("What is Y?", true, true)
	if !strings.HasPrefix(next, "---\n\n") {
		t.Errorf("changed-prompt marker should start with a separator: %q", next)
	}
	if !strings.Contains(next, "Response to: What is Y?") {
		t.Errorf("marker missing prompt: %q", next)
	}
}
