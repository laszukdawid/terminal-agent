package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/app"
)

const taskLiveOutputMaxLineChars = 120

type taskLiveOutputPrinter struct {
	out                   io.Writer
	progress              *taskProgressPrinter
	limiter               *taskLiveOutputLimiter
	command               string
	commandPrinted        bool
	printed               bool
	lastChunkEndedNewline bool
}

func newTaskLiveOutputPrinter(out io.Writer, progress *taskProgressPrinter, maxLines int) *taskLiveOutputPrinter {
	if out == nil {
		out = io.Discard
	}
	return &taskLiveOutputPrinter{
		out:                   out,
		progress:              progress,
		limiter:               newTaskLiveOutputLimiter(maxLines, taskLiveOutputMaxLineChars),
		lastChunkEndedNewline: true,
	}
}

func (p *taskLiveOutputPrinter) BeginTool(event app.Event) {
	p.limiter.Reset()
	p.command = formatTaskStreamedCommand(event)
	p.commandPrinted = false
}

func (p *taskLiveOutputPrinter) PrintDelta(text string) {
	chunk := p.limiter.Filter(text)
	if chunk == "" {
		return
	}

	if p.progress != nil {
		p.progress.Clear()
	}
	if p.command != "" && !p.commandPrinted {
		if p.printed && !p.lastChunkEndedNewline {
			_, _ = fmt.Fprint(p.out, "\n")
		}
		_, _ = fmt.Fprintf(p.out, "Command: %s\n", p.command)
		p.commandPrinted = true
	}
	p.printed = true
	p.lastChunkEndedNewline = strings.HasSuffix(chunk, "\n")
	_, _ = fmt.Fprint(p.out, chunk)
}

func (p *taskLiveOutputPrinter) Printed() bool {
	return p.printed
}

func (p *taskLiveOutputPrinter) NeedsNewlineBeforeFinal() bool {
	return p.printed && !p.lastChunkEndedNewline
}

type taskLiveOutputLimiter struct {
	maxLines            int
	maxLineChars        int
	lines               int
	charsWithoutNewline int
	sawNewline          bool
	truncated           bool
	truncationAnnounced bool
}

func newTaskLiveOutputLimiter(maxLines int, maxLineChars int) *taskLiveOutputLimiter {
	return &taskLiveOutputLimiter{maxLines: maxLines, maxLineChars: maxLineChars}
}

func (l *taskLiveOutputLimiter) Reset() {
	l.lines = 0
	l.charsWithoutNewline = 0
	l.sawNewline = false
	l.truncated = false
	l.truncationAnnounced = false
}

func (l *taskLiveOutputLimiter) Filter(chunk string) string {
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

func formatLiveOutputTruncation(shown int, totalLines int, lineMode bool) string {
	if lineMode && totalLines > shown {
		return fmt.Sprintf("[tool output truncated: %d out of %d lines]\n", shown, totalLines)
	}
	return fmt.Sprintf("[tool output truncated: %d lines shown]\n", shown)
}

func formatTaskStreamedCommand(event app.Event) string {
	action := agent.BuildActionString(event.ToolName, event.ToolInput)
	_, display := agent.ParseToolAndDisplay(action)
	if display != "" {
		return display
	}
	return action
}
