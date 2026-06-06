package commands

import (
	"fmt"
	"io"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/tools"
)

const taskLiveOutputMaxLineChars = 120

type toolOutputState struct {
	printed   bool
	truncated bool
}

type taskLiveOutputPrinter struct {
	out                   io.Writer
	progress              *taskProgressPrinter
	limiter               *taskLiveOutputLimiter
	defaultMaxLines       int
	toolName              string
	currentTool           string
	toolStates            map[string]toolOutputState
	command               string
	commandPrinted        bool
	traceStyling          bool
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
		defaultMaxLines:       maxLines,
		toolStates:            make(map[string]toolOutputState),
		traceStyling:          isTerminalWriter(out),
		lastChunkEndedNewline: true,
	}
}

func (p *taskLiveOutputPrinter) BeginTool(event app.Event) {
	p.snapshotCurrentTool()
	p.currentTool = event.ToolName

	isFinal, _ := event.ToolInput["final"].(bool)
	if isFinal {
		p.limiter.ResetWithMaxLines(0)
	} else {
		p.limiter.ResetWithMaxLines(p.defaultMaxLines)
	}
	p.toolName, p.command = formatTaskStreamedCommand(event)
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
		_, _ = fmt.Fprint(p.out, formatToolCommandTrace(p.toolName, p.command, p.traceStyling))
		p.commandPrinted = true
	}
	p.printed = true
	if p.currentTool != "" {
		state := p.toolStates[p.currentTool]
		state.printed = true
		p.toolStates[p.currentTool] = state
	}
	p.lastChunkEndedNewline = strings.HasSuffix(chunk, "\n")
	_, _ = fmt.Fprint(p.out, chunk)
}

func (p *taskLiveOutputPrinter) Printed() bool {
	return p.printed
}

func (p *taskLiveOutputPrinter) Truncated() bool {
	return p.limiter.truncated
}

func (p *taskLiveOutputPrinter) snapshotCurrentTool() {
	if p.currentTool == "" {
		return
	}
	state := p.toolStates[p.currentTool]
	state.truncated = state.truncated || p.limiter.truncated
	p.toolStates[p.currentTool] = state
}

func (p *taskLiveOutputPrinter) PrintedTool(name string) bool {
	p.snapshotCurrentTool()
	return p.toolStates[name].printed
}

func (p *taskLiveOutputPrinter) TruncatedTool(name string) bool {
	p.snapshotCurrentTool()
	return p.toolStates[name].truncated
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

func (l *taskLiveOutputLimiter) ResetWithMaxLines(maxLines int) {
	l.maxLines = maxLines
	l.Reset()
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

func formatTaskStreamedCommand(event app.Event) (toolName, display string) {
	action := agent.BuildActionString(event.ToolName, event.ToolInput)
	toolName, display = agent.ParseToolAndDisplay(action)
	if display != "" {
		return toolName, display
	}
	return toolName, action
}

func formatToolCommandTrace(toolName string, command string, styled bool) string {
	if toolName == tools.ToolNamePython {
		return formatPythonTrace(command, styled)
	}
	return formatTaskTrace("Command", command, styled) + "\n"
}

func formatPythonTrace(code string, styled bool) string {
	var out strings.Builder
	if styled {
		fmt.Fprintf(&out, "%sPython:%s\n", taskTraceANSIBoldCyan, taskTraceANSIReset)
	} else {
		out.WriteString("Python:\n")
	}
	for _, line := range strings.Split(code, "\n") {
		out.WriteString("  ")
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}
