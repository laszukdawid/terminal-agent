package gui

import (
	"strings"
	"testing"
)

func TestLiveOutputLimiterUnlimited(t *testing.T) {
	l := newLiveOutputLimiter(0, liveOutputMaxLineChars)
	in := "a\nb\nc\nd\ne\n"
	if got := l.Filter(in); got != in {
		t.Fatalf("unlimited limiter altered output: %q", got)
	}
}

func TestLiveOutputLimiterTruncatesByLines(t *testing.T) {
	l := newLiveOutputLimiter(2, liveOutputMaxLineChars)
	got := l.Filter("l1\nl2\nl3\nl4\n")
	if !strings.Contains(got, "l1") || !strings.Contains(got, "l2") {
		t.Fatalf("first 2 lines should be kept: %q", got)
	}
	if strings.Contains(got, "l3") {
		t.Fatalf("lines beyond the limit should be dropped: %q", got)
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("expected a truncation marker: %q", got)
	}
	if next := l.Filter("more\n"); next != "" {
		t.Fatalf("input after truncation must be dropped, got %q", next)
	}
}
