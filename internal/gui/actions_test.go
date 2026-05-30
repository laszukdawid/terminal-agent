package gui

import (
	"strings"
	"testing"
)

func TestResponseMarker(t *testing.T) {
	// First response (no previous output): label only, no separator.
	first := responseMarker("What is X?", false)
	if !strings.Contains(first, "Response to: What is X?") {
		t.Errorf("marker missing prompt: %q", first)
	}
	if strings.Contains(first, "---") {
		t.Errorf("first response should not have a separator: %q", first)
	}

	// Replacing a previous response (prompt changed): separator is prefixed.
	next := responseMarker("What is Y?", true)
	if !strings.HasPrefix(next, "---\n\n") {
		t.Errorf("changed-prompt marker should start with a separator: %q", next)
	}
	if !strings.Contains(next, "Response to: What is Y?") {
		t.Errorf("marker missing prompt: %q", next)
	}
}
