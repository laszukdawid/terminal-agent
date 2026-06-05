package commands

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/history"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var newService = app.NewService

func NewTaskCommand(config config.Config) *cobra.Command {
	var provider *string
	var modelID *string
	var promptFlag *string
	var allowList *[]string

	cmd := &cobra.Command{
		Use:          "task",
		Short:        "Execute a task using the underlying LLM model",
		SilenceUsage: true,
		Long: `Execute a task using the underlying LLM model

		Any remaining argument that isn't captured by the flags will be concatenated to form the query.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := cmd.Flags()
			service := newService()
			inputReader := bufio.NewReader(cmd.InOrStdin())
			device, err := resolveDevice(flags, config)
			if err != nil {
				return err
			}
			taskWorkingDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to resolve task working directory: %w", err)
			}

			// Concatenate all remaining args to form the query
			userRequest := strings.Join(args, " ")

			allow := []string{}
			if allowList != nil {
				allow = *allowList
			}

			// Resolve the task timeout: an explicit --timeout (including 0 =
			// unlimited) wins; otherwise fall back to the configured default.
			taskTimeout := config.GetTaskTimeout()
			if flags.Changed("timeout") {
				if flagTimeout, err := flags.GetDuration("timeout"); err == nil {
					taskTimeout = flagTimeout
				}
			}
			autoApprove, err := flags.GetBool("auto-approve")
			if err != nil {
				autoApprove = false
			}

			events, err := service.TaskEvents(ctx, app.TaskRequest{
				Message:        userRequest,
				Provider:       *provider,
				Model:          *modelID,
				PromptOverride: *promptFlag,
				WorkingDir:     taskWorkingDir,
				Allow:          allow,
				AutoApprove:    autoApprove,
				Device:         device,
				Timeout:        taskTimeout,
				Config:         config,
			})
			if err != nil {
				return fmt.Errorf("failed to request a task: %w", err)
			}

			printFlag, printFlagErr := flags.GetBool("print")
			if printFlagErr != nil {
				printFlag = true
			}
			progressMode, err := flags.GetString("progress")
			if err != nil {
				progressMode = "auto"
			}
			progressConfig, err := resolveTaskProgress(progressMode, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			progress := newTaskProgressPrinter(cmd.ErrOrStderr(), progressConfig)
			defer progress.Clear()
			liveOutput := newTaskLiveOutputLimiter(config.GetTaskLiveOutputLimit(), 120)

			result := app.TaskResult{Request: userRequest}
			printedStreamedToolOutput := false
			lastStreamChunkEndedWithNewline := true
			for event := range events {
				switch event.Type {
				case app.EventTaskStatus:
					progress.Print(event.Text)
				case app.EventToolProgress:
					progress.Print(formatToolProgress(event))
				case app.EventOutputDelta:
					if printFlag {
						chunk := liveOutput.Filter(event.Text)
						if chunk != "" {
							progress.Clear()
							printedStreamedToolOutput = true
							lastStreamChunkEndedWithNewline = strings.HasSuffix(chunk, "\n")
							cmd.Print(chunk)
						}
					}
				case app.EventWarning:
					if event.Text != "" {
						progress.Clear()
						cmd.PrintErrf("Warning: %s\n", event.Text)
					}
				case app.EventConfirmationNeeded:
					progress.Clear()
					decision, promptErr := promptTaskConfirmation(cmd, inputReader, event.Confirmation)
					if promptErr != nil {
						return promptErr
					}
					if replyErr := event.Confirmation.Reply(decision); replyErr != nil {
						return replyErr
					}
				case app.EventClarificationNeeded:
					progress.Clear()
					answer, promptErr := promptTaskClarification(cmd, inputReader, event.Clarification)
					if promptErr != nil {
						return promptErr
					}
					if replyErr := event.Clarification.Reply(answer); replyErr != nil {
						return replyErr
					}
				case app.EventCompleted:
					result.Response = event.FinalOutput
					if !printedStreamedToolOutput {
						result.RawOutput = event.RawOutput
					}
					result.RawOutputTool = event.RawOutputTool
					result.DirectRawOutput = event.DirectRawOutput
				case app.EventFailed:
					progress.Clear()
					return fmt.Errorf("failed to request a task: %w", event.Err)
				}
			}
			progress.Clear()

			response := result.Response

			if printedStreamedToolOutput && result.DirectRawOutput {
				response = ""
			}

			if printFlag && response != "" {
				if printedStreamedToolOutput && !lastStreamChunkEndedWithNewline {
					cmd.Print("\n")
				}
				plain, _ := flags.GetBool("plain")
				cmd.Print(formatTaskOutput(app.TaskResult{
					Response:        response,
					RawOutput:       result.RawOutput,
					RawOutputTool:   result.RawOutputTool,
					DirectRawOutput: result.DirectRawOutput,
				}, plain))
			}

			if logFlag, err := flags.GetBool("log"); logFlag && err == nil {
				hClient := history.NewHistory(getLogPath())
				hClient.Log("task", result.Request, response)
			}

			return nil
		},
		Args: cobra.MinimumNArgs(1),
	}

	provider = cmd.Flags().StringP("provider", "p", config.GetDefaultProvider(), "The provider to use for the question")
	modelID = cmd.Flags().StringP("model", "m", config.GetDefaultModelId(), "The model ID to use for the question")
	promptFlag = cmd.Flags().String("prompt", "", "Custom system prompt (overrides file-based and default prompts)")
	allowList = cmd.Flags().StringArray("allow", []string{}, "Allow exact action without confirmation (repeatable)")
	cmd.Flags().Bool("auto-approve", false, "Automatically approve confirmation prompts except explicit denies")

	// 'timeout' flag bounds the whole task run (Go duration, e.g. 15m). 0 means unlimited.
	// Defaults to unlimited unless task_timeout is set in config.
	cmd.Flags().Duration("timeout", 0, "Maximum task duration (e.g. 90s, 15m, 2h); 0 means no timeout")

	// 'print' flag whether to print response to the stdout (default: true)
	cmd.Flags().BoolP("print", "x", true, "Print the response to the stdout")

	// 'plain' flag whether to render the response as plain text (default: false)
	cmd.Flags().BoolP("plain", "k", false, "Render the response as plain text")

	// 'log' flag whether to log the input and output to a file (default: false)
	cmd.Flags().BoolP("log", "l", false, "Log the input and output to a file")

	cmd.Flags().String("progress", "auto", "Show task progress on stderr: auto, always, or never")
	return cmd
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

type taskProgressPrinter struct {
	enabled     bool
	interactive bool
	out         io.Writer

	mu      sync.Mutex
	message string
	frame   int
	active  bool
	stop    chan struct{}
	done    chan struct{}
}

type taskProgressConfig struct {
	enabled     bool
	interactive bool
}

var taskProgressFrames = []string{"|", "/", "-", "\\"}

func newTaskProgressPrinter(out io.Writer, config taskProgressConfig) *taskProgressPrinter {
	return &taskProgressPrinter{enabled: config.enabled, interactive: config.interactive, out: out}
}

func (p *taskProgressPrinter) Print(message string) {
	if !p.enabled || p.out == nil {
		return
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}

	p.mu.Lock()
	if message == p.message {
		p.mu.Unlock()
		return
	}
	p.message = message
	if !p.interactive {
		p.mu.Unlock()
		_, _ = fmt.Fprintln(p.out, message)
		return
	}
	if !p.active {
		p.active = true
		p.stop = make(chan struct{})
		p.done = make(chan struct{})
		go p.animate(p.stop, p.done)
	}
	p.renderLocked()
	p.mu.Unlock()
}

func (p *taskProgressPrinter) Clear() {
	if !p.enabled || p.out == nil || !p.interactive {
		return
	}

	p.mu.Lock()
	if !p.active {
		p.mu.Unlock()
		return
	}
	stop := p.stop
	done := p.done
	p.active = false
	p.stop = nil
	p.done = nil
	p.message = ""
	close(stop)
	p.clearLineLocked()
	p.mu.Unlock()
	<-done
}

func (p *taskProgressPrinter) animate(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			if p.active && p.message != "" {
				p.frame++
				p.renderLocked()
			}
			p.mu.Unlock()
		case <-stop:
			return
		}
	}
}

func (p *taskProgressPrinter) renderLocked() {
	frame := taskProgressFrames[p.frame%len(taskProgressFrames)]
	_, _ = fmt.Fprintf(p.out, "\r\033[2K%s %s", frame, p.message)
}

func (p *taskProgressPrinter) clearLineLocked() {
	_, _ = fmt.Fprint(p.out, "\r\033[2K")
}

func formatToolProgress(event app.Event) string {
	message := strings.TrimSpace(event.Text)
	if message == "" {
		return ""
	}
	if event.ToolName == "" {
		return message
	}
	return fmt.Sprintf("%s: %s", event.ToolName, message)
}

func resolveTaskProgress(mode string, stderr io.Writer) (taskProgressConfig, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "auto":
		file, ok := stderr.(*os.File)
		interactive := ok && term.IsTerminal(int(file.Fd()))
		return taskProgressConfig{enabled: interactive, interactive: interactive}, nil
	case "always":
		file, ok := stderr.(*os.File)
		interactive := ok && term.IsTerminal(int(file.Fd()))
		return taskProgressConfig{enabled: true, interactive: interactive}, nil
	case "never":
		return taskProgressConfig{}, nil
	default:
		return taskProgressConfig{}, fmt.Errorf("invalid --progress value %q; expected auto, always, or never", mode)
	}
}

func formatTaskOutput(result app.TaskResult, plain bool) string {
	response := result.Response
	if result.DirectRawOutput {
		if !strings.HasSuffix(response, "\n") {
			return response + "\n"
		}
		return response
	}
	if !plain {
		response = handleMarkdown(response)
	}

	if result.RawOutput == "" {
		return response
	}

	var output strings.Builder
	if response != "" {
		output.WriteString(response)
		if !strings.HasSuffix(response, "\n") {
			output.WriteByte('\n')
		}
		output.WriteByte('\n')
	}

	toolName := result.RawOutputTool
	if toolName == "" {
		toolName = "tool"
	}
	output.WriteString(fmt.Sprintf("Raw output from %s:\n", toolName))
	output.WriteString(result.RawOutput)
	if !strings.HasSuffix(result.RawOutput, "\n") {
		output.WriteByte('\n')
	}

	return output.String()
}

func promptTaskConfirmation(cmd *cobra.Command, reader *bufio.Reader, confirmation *app.TaskConfirmationEvent) (app.TaskConfirmationResponse, error) {
	if confirmation == nil {
		return app.TaskConfirmationResponse{}, fmt.Errorf("missing confirmation request")
	}

	stdinFile, stdinOk := cmd.InOrStdin().(*os.File)
	stderrFile, stderrOk := cmd.ErrOrStderr().(*os.File)
	if stdinOk && stderrOk && term.IsTerminal(int(stdinFile.Fd())) && term.IsTerminal(int(stderrFile.Fd())) {
		return promptTaskConfirmationInteractive(stdinFile, stderrFile, confirmation)
	}

	return promptTaskConfirmationLine(cmd, reader, confirmation)
}

func promptTaskConfirmationInteractive(stdin *os.File, stderr *os.File, confirmation *app.TaskConfirmationEvent) (app.TaskConfirmationResponse, error) {
	ic := newInteractiveConfirmation(confirmation.Action, stdin, stderr)
	result, err := ic.run()
	if err != nil {
		return app.TaskConfirmationResponse{}, err
	}

	resp, err := processConfirmationResponse(result.response, result.pattern)
	if err != nil {
		return resp, err
	}

	if resp.Allowed {
		display := ic.command
		if display == "" {
			display = ic.action
		}
		fmt.Fprintf(stderr, "Executing: %s\n", display)
	}

	return resp, nil
}

func promptTaskConfirmationLine(cmd *cobra.Command, reader *bufio.Reader, confirmation *app.TaskConfirmationEvent) (app.TaskConfirmationResponse, error) {
	toolName, command := agent.ParseToolAndDisplay(confirmation.Action)
	display := confirmation.Action
	header := "Execute action?"
	if command != "" {
		display = command
		switch toolName {
		case tools.ToolNameUnix:
			header = "Run shell command?"
		case tools.ToolNamePython:
			header = "Run Python script?"
		}
	}
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "%s\n%s\n[y/N/a/b]: ", header, indentDisplayLines(display)); err != nil {
		return app.TaskConfirmationResponse{}, err
	}

	response, err := reader.ReadString('\n')
	if err != nil {
		return app.TaskConfirmationResponse{}, err
	}

	return processConfirmationResponse(strings.TrimSpace(response), confirmation.Action)
}

func indentDisplayLines(display string) string {
	var lines []string
	for _, line := range splitDisplayLines(display) {
		lines = append(lines, "  "+line)
	}
	return strings.Join(lines, "\n")
}

func processConfirmationResponse(response string, actionPattern string) (app.TaskConfirmationResponse, error) {
	switch strings.TrimSpace(strings.ToLower(response)) {
	case "y", "yes":
		return app.TaskConfirmationResponse{Allowed: true}, nil
	case "a":
		return app.TaskConfirmationResponse{Allowed: true, Remember: true, Patterns: []string{actionPattern}}, nil
	case "b":
		return app.TaskConfirmationResponse{Remember: true, Patterns: []string{actionPattern}}, nil
	default:
		return app.TaskConfirmationResponse{}, nil
	}
}

func promptTaskClarification(cmd *cobra.Command, reader *bufio.Reader, clarification *app.TaskClarificationEvent) (string, error) {
	if clarification == nil {
		return "", fmt.Errorf("missing clarification request")
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "\nNeed clarification: %s\n> ", clarification.Question); err != nil {
		return "", err
	}

	response, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimRight(response, "\r\n"), nil
}
