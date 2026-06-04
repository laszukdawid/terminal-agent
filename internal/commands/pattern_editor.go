package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/agent"
	"golang.org/x/term"
)

var errPromptCancelled = errors.New("prompt cancelled")

type confirmationResult struct {
	response string
	pattern  string
}

type interactiveConfirmation struct {
	toolName        string
	command         string
	action          string
	levels          []string
	pos             int
	writer          io.Writer
	showHelp        bool
	termWidth       int
	lastVisualLines int
}

func newInteractiveConfirmation(action string, writer io.Writer) *interactiveConfirmation {
	toolName, command := agent.ParseToolAndCommand(action)

	var levels []string
	if toolName != "" && command != "" {
		groups := agent.TokenizeCommand(command)
		levels = agent.GeneratePatternLevels(groups)
	}

	width, _, _ := term.GetSize(int(os.Stdin.Fd()))
	if width <= 0 {
		width = 80
	}

	return &interactiveConfirmation{
		toolName:  toolName,
		command:   command,
		action:    action,
		levels:    levels,
		pos:       0,
		writer:    writer,
		termWidth: width,
	}
}

func (c *interactiveConfirmation) run() (confirmationResult, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return confirmationResult{}, err
	}
	defer term.Restore(fd, oldState)

	c.render()

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			c.cleanup()
			return confirmationResult{}, err
		}

		if n == 3 && buf[0] == 0x1b && buf[1] == '[' {
			switch buf[2] {
			case 'D': // left arrow — broaden
				if c.pos < len(c.levels)-1 {
					c.pos++
					c.showHelp = false
					c.render()
				}
			case 'C': // right arrow — narrow
				if c.pos > 0 {
					c.pos--
					c.showHelp = false
					c.render()
				}
			}
			continue
		}

		if n == 1 {
			switch buf[0] {
			case 'y', 'Y':
				c.cleanup()
				return confirmationResult{response: "y", pattern: c.currentAction()}, nil
			case 'a', 'A':
				c.cleanup()
				return confirmationResult{response: "a", pattern: c.currentAction()}, nil
			case 'b', 'B':
				c.cleanup()
				return confirmationResult{response: "b", pattern: c.currentAction()}, nil
			case 'n', 'N', '\r', '\n':
				c.cleanup()
				return confirmationResult{response: "n", pattern: c.currentAction()}, nil
			case '?':
				c.showHelp = !c.showHelp
				c.render()
			case 0x03: // ctrl-c
				c.cleanup()
				return confirmationResult{}, errPromptCancelled
			case 0x1b: // lone escape — ignore
			}
		}
	}
}

func (c *interactiveConfirmation) currentAction() string {
	if len(c.levels) == 0 || c.pos < 0 || c.pos >= len(c.levels) {
		return c.action
	}
	return fmt.Sprintf("%s(%s)", c.toolName, quotePatternValue(c.levels[c.pos]))
}

func (c *interactiveConfirmation) hasMultipleLevels() bool {
	return len(c.levels) > 1
}

func (c *interactiveConfirmation) currentDisplayCommand() string {
	if len(c.levels) == 0 || c.pos < 0 || c.pos >= len(c.levels) {
		if c.command != "" {
			return c.command
		}
		return c.action
	}
	return c.levels[c.pos]
}

func (c *interactiveConfirmation) headerText() string {
	switch c.toolName {
	case "unix":
		return "Run shell command?"
	case "python":
		return "Run Python script?"
	default:
		return "Execute action?"
	}
}

func (c *interactiveConfirmation) render() {
	// Move cursor back to the start of our block and clear everything below.
	if c.lastVisualLines > 0 {
		for i := 0; i < c.lastVisualLines-1; i++ {
			fmt.Fprintf(c.writer, "\x1b[1A")
		}
		fmt.Fprintf(c.writer, "\r\x1b[J")
	}

	var lines []string
	lines = append(lines, c.headerText())
	lines = append(lines, "")
	lines = append(lines, "  "+c.currentDisplayCommand())
	lines = append(lines, "")
	if c.hasMultipleLevels() {
		lines = append(lines, "  Use arrows: ← broader  narrower →")
	}
	lines = append(lines, "[y] allow once  [Enter/N] deny  [a] always allow…  [b] always block…  [?] help")

	if c.showHelp {
		lines = append(lines, "")
		lines = append(lines, "  y        allow this action once")
		lines = append(lines, "  Enter/N  deny this action (default)")
		lines = append(lines, "  a        always allow actions matching current pattern")
		lines = append(lines, "  b        always block actions matching current pattern")
		if c.hasMultipleLevels() {
			lines = append(lines, "  ←/→      adjust pattern scope (broader ↔ narrower)")
		}
	}

	totalVisual := 0
	for _, line := range lines {
		totalVisual += c.visualLineCount(line)
	}
	c.lastVisualLines = totalVisual

	fmt.Fprintf(c.writer, "%s", strings.Join(lines, "\r\n"))
}

func (c *interactiveConfirmation) visualLineCount(s string) int {
	if c.termWidth <= 0 || len(s) == 0 {
		return 1
	}
	return (len(s) + c.termWidth - 1) / c.termWidth
}

func (c *interactiveConfirmation) cleanup() {
	if c.lastVisualLines > 0 {
		for i := 0; i < c.lastVisualLines-1; i++ {
			fmt.Fprintf(c.writer, "\x1b[1A")
		}
		fmt.Fprintf(c.writer, "\r\x1b[J")
	}
}

// PatternEditor is kept for tests and as a building block. In production,
// interactiveConfirmation handles the full prompt+editor flow.
type PatternEditor struct {
	toolName string
	command  string
	levels   []string
	pos      int
	writer   io.Writer
}

func NewPatternEditor(toolName, command string, writer io.Writer) *PatternEditor {
	groups := agent.TokenizeCommand(command)
	levels := agent.GeneratePatternLevels(groups)

	return &PatternEditor{
		toolName: toolName,
		command:  command,
		levels:   levels,
		pos:      0,
		writer:   writer,
	}
}

func (e *PatternEditor) Run() (string, error) {
	if len(e.levels) <= 1 {
		return e.buildAction(e.currentPattern()), nil
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return e.buildAction(e.command), nil
	}
	defer term.Restore(fd, oldState)

	e.render()

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return e.buildAction(e.command), nil
		}

		if n == 1 {
			switch buf[0] {
			case '\r', '\n':
				e.clearLine()
				return e.buildAction(e.currentPattern()), nil
			case 0x1b:
				e.clearLine()
				return e.buildAction(e.command), nil
			case 0x03:
				e.clearLine()
				return e.buildAction(e.command), nil
			}
		}

		if n == 3 && buf[0] == 0x1b && buf[1] == '[' {
			switch buf[2] {
			case 'D':
				if e.pos < len(e.levels)-1 {
					e.pos++
					e.render()
				}
			case 'C':
				if e.pos > 0 {
					e.pos--
					e.render()
				}
			}
		}
	}
}

func (e *PatternEditor) currentPattern() string {
	if e.pos < 0 || e.pos >= len(e.levels) {
		return e.command
	}
	return e.levels[e.pos]
}

func (e *PatternEditor) buildAction(pattern string) string {
	return fmt.Sprintf("%s(%s)", e.toolName, quotePatternValue(pattern))
}

func (e *PatternEditor) render() {
	action := e.buildAction(e.currentPattern())
	hint := "← → to adjust, Enter to save, Esc for exact"
	e.clearLine()
	fmt.Fprintf(e.writer, "\rSave as: %s\r\n         %s", action, hint)
}

func (e *PatternEditor) clearLine() {
	fmt.Fprintf(e.writer, "\x1b[1A\r\x1b[K\r\x1b[1B\x1b[K\x1b[1A")
}

func quotePatternValue(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"\n", "\\n",
		"\r", "\\r",
		"\t", "\\t",
	)
	return fmt.Sprintf("\"%s\"", replacer.Replace(value))
}
