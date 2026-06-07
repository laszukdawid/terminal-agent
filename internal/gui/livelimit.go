package gui

import (
	"fmt"
	"strings"
)

// liveOutputMaxLineChars caps a single newline-free run of output so one giant
// line cannot defeat the line-based limit. Mirrors the CLI value.
const liveOutputMaxLineChars = 120

// liveOutputLimiter bounds live tool output to maxLines lines (0 = unlimited),
// emitting a one-time truncation marker once the limit is hit. It is a port of
// the CLI taskLiveOutputLimiter (internal/commands/task_live_output.go) so the
// GUI and CLI truncate identically. The proper fix (a segmented transcript that
// renders tool output as monospace text rather than markdown) is tracked in
// GitHub issue #55.
type liveOutputLimiter struct {
	maxLines            int
	maxLineChars        int
	lines               int
	charsWithoutNewline int
	sawNewline          bool
	truncated           bool
	truncationAnnounced bool
}

func newLiveOutputLimiter(maxLines, maxLineChars int) *liveOutputLimiter {
	return &liveOutputLimiter{maxLines: maxLines, maxLineChars: maxLineChars}
}

// Truncated reports whether the limiter has dropped any output. Used so
// completion can still surface the full raw result when a tool's live output
// was capped (mirrors the CLI's TruncatedTool de-dupe).
func (l *liveOutputLimiter) Truncated() bool {
	return l.truncated
}

// Filter returns the portion of chunk that should be appended, including a
// one-time truncation marker when the limit is first exceeded. After the marker
// is emitted, all further input returns "".
func (l *liveOutputLimiter) Filter(chunk string) string {
	if chunk == "" || l.truncationAnnounced {
		return ""
	}
	if l.maxLines == 0 {
		return chunk
	}

	var out strings.Builder
	startLines := l.lines
	knownChunkLines := countOutputLines(chunk)
	for _, r := range chunk {
		if l.lines >= l.maxLines || (!l.sawNewline && l.charsWithoutNewline >= l.maxLines*l.maxLineChars) {
			l.truncated = true
			break
		}
		out.WriteRune(r)
		if r == '\n' {
			l.sawNewline = true
			l.lines++
			continue
		}
		if !l.sawNewline {
			l.charsWithoutNewline++
		}
	}

	if l.truncated {
		l.truncationAnnounced = true
		if out.Len() > 0 && !strings.HasSuffix(out.String(), "\n") {
			out.WriteByte('\n')
		}
		out.WriteString(formatLiveOutputTruncation(l.maxLines, startLines+knownChunkLines, l.sawNewline))
	}
	return out.String()
}

func countOutputLines(s string) int {
	if s == "" {
		return 0
	}
	lines := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		lines++
	}
	return lines
}

func formatLiveOutputTruncation(shown, totalLines int, lineMode bool) string {
	if lineMode && totalLines > shown {
		return fmt.Sprintf("[tool output truncated: %d out of %d lines]\n", shown, totalLines)
	}
	return fmt.Sprintf("[tool output truncated: %d lines shown]\n", shown)
}
