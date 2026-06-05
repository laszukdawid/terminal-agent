package commands

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTaskService struct {
	events func(context.Context, app.TaskRequest) (<-chan app.Event, error)
}

func (s *fakeTaskService) Ask(context.Context, app.AskRequest) (app.AskResult, error) {
	panic("unexpected Ask call")
}

func (s *fakeTaskService) AskEvents(context.Context, app.AskRequest) (<-chan app.Event, error) {
	panic("unexpected AskEvents call")
}

func (s *fakeTaskService) Chat(context.Context, app.ChatRequest) (app.ChatResult, error) {
	panic("unexpected Chat call")
}

func (s *fakeTaskService) ChatEvents(context.Context, app.ChatRequest) (<-chan app.Event, error) {
	panic("unexpected ChatEvents call")
}

func (s *fakeTaskService) TaskEvents(ctx context.Context, req app.TaskRequest) (<-chan app.Event, error) {
	return s.events(ctx, req)
}

func TestFormatTaskOutput(t *testing.T) {
	t.Run("plain response only", func(t *testing.T) {
		output := formatTaskOutput(app.TaskResult{Response: "done"}, true)
		assert.Equal(t, "done", output)
	})

	t.Run("direct raw output", func(t *testing.T) {
		output := formatTaskOutput(app.TaskResult{
			Response:        "drwxr-xr-x\t2 user staff\n-rw-r--r--\t1 user staff",
			RawOutputTool:   tools.ToolNameUnix,
			RawOutput:       "drwxr-xr-x\t2 user staff\n-rw-r--r--\t1 user staff",
			DirectRawOutput: true,
		}, false)

		assert.Equal(t, "drwxr-xr-x\t2 user staff\n-rw-r--r--\t1 user staff\n", output)
	})

	t.Run("plain response with raw output", func(t *testing.T) {
		output := formatTaskOutput(app.TaskResult{
			Response:      "Here are the files.",
			RawOutputTool: tools.ToolNameUnix,
			RawOutput:     "drwxr-xr-x\t2 user staff\n-rw-r--r--\t1 user staff",
		}, true)

		assert.Contains(t, output, "Here are the files.\n\n")
		assert.Contains(t, output, "Raw output from "+tools.ToolNameUnix+":\n")
		assert.Contains(t, output, "drwxr-xr-x\t2 user staff\n-rw-r--r--\t1 user staff")
	})
}

func TestPromptTaskConfirmationLineDisplaysToolActions(t *testing.T) {
	tests := []struct {
		name        string
		action      string
		wantHeader  string
		wantDisplay string
	}{
		{
			name:        "python code",
			action:      tools.ToolNamePython + `(code="print(\"hello\")\nprint(\"world\")")`,
			wantHeader:  "Run Python script?",
			wantDisplay: "  print(\"hello\")\n  print(\"world\")",
		},
		{
			name:        "unix command",
			action:      tools.ToolNameUnix + `("git status")`,
			wantHeader:  "Run shell command?",
			wantDisplay: "  git status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewTaskCommand(config.NewDefaultConfig())
			input := bytes.NewBufferString("y\n")
			output := &bytes.Buffer{}
			cmd.SetErr(output)

			decision, err := promptTaskConfirmationLine(cmd, bufio.NewReader(input), &app.TaskConfirmationEvent{Action: tt.action})

			require.NoError(t, err)
			assert.True(t, decision.Allowed)
			text := output.String()
			assert.Contains(t, text, tt.wantHeader)
			assert.Contains(t, text, tt.wantDisplay)
			assert.NotContains(t, text, tools.ToolNamePython+`(code=`)
		})
	}
}

func TestTaskCommandHandlesInteractiveEvents(t *testing.T) {
	originalNewService := newService
	defer func() {
		newService = originalNewService
	}()

	var confirmation app.TaskConfirmationResponse
	var clarification string
	newService = func() app.Service {
		return &fakeTaskService{events: func(_ context.Context, req app.TaskRequest) (<-chan app.Event, error) {
			ch := make(chan app.Event)
			go func() {
				defer close(ch)

				confirmed := make(chan struct{})
				ch <- app.Event{
					Type: app.EventConfirmationNeeded,
					Confirmation: &app.TaskConfirmationEvent{
						Action: tools.ToolNameUnix + `("git status")`,
						Reply: func(response app.TaskConfirmationResponse) error {
							confirmation = response
							close(confirmed)
							return nil
						},
					},
				}
				<-confirmed

				clarified := make(chan struct{})
				ch <- app.Event{
					Type: app.EventClarificationNeeded,
					Clarification: &app.TaskClarificationEvent{
						Question: "Which directory?",
						Reply: func(response string) error {
							clarification = response
							close(clarified)
							return nil
						},
					},
				}
				<-clarified

				ch <- app.Event{Type: app.EventCompleted, FinalOutput: "done", Status: req.Message}
			}()
			return ch, nil
		}}
	}

	cmd := NewTaskCommand(config.NewDefaultConfig())
	input := bytes.NewBufferString("a\ninternal\n")
	output := &bytes.Buffer{}
	cmd.SetIn(input)
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.Flags().String("device", "", "")
	cmd.SetArgs([]string{"inspect", "repo"})

	err := cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	assert.Equal(t, app.TaskConfirmationResponse{Allowed: true, Remember: true, Patterns: []string{tools.ToolNameUnix + `("git status")`}}, confirmation)
	assert.Equal(t, "internal", clarification)
	assert.Contains(t, output.String(), "Run shell command?")
	assert.Contains(t, output.String(), "Need clarification: Which directory?")
	assert.Contains(t, output.String(), "done")
}

func TestTaskCommandStreamedOutput(t *testing.T) {
	tests := []struct {
		name        string
		cfg         config.Config
		args        []string
		events      []app.Event
		want        string
		contains    []string
		notContains []string
	}{
		{
			name:   "suppresses streamed output when print disabled",
			args:   []string{"--print=false", "inspect", "repo"},
			events: []app.Event{{Type: app.EventOutputDelta, Text: "live output"}},
			want:   "",
		},
		{
			name:   "separates streamed output from final response",
			args:   []string{"--plain", "inspect", "repo"},
			events: []app.Event{{Type: app.EventOutputDelta, Text: "live output"}},
			want:   "live output\ndone",
		},
		{
			name:        "limits streamed output by default",
			args:        []string{"--plain", "inspect", "repo"},
			events:      []app.Event{{Type: app.EventOutputDelta, Text: "1\n2\n3\n4\n5\n6\n7\n8\n"}},
			contains:    []string{"1\n2\n3\n4\n5\n6\n", "[tool output truncated: 6 out of 8 lines]\n", "done"},
			notContains: []string{"7\n"},
		},
		{
			name:        "uses configured streamed output limit",
			cfg:         taskLiveOutputLimitConfig(2),
			args:        []string{"--plain", "inspect", "repo"},
			events:      []app.Event{{Type: app.EventOutputDelta, Text: "1\n2\n3\n"}},
			contains:    []string{"1\n2\n", "tool output truncated: 2 out of 3 lines"},
			notContains: []string{"3\n"},
		},
		{
			name:        "supports unlimited streamed output",
			cfg:         taskLiveOutputLimitConfig(0),
			args:        []string{"--plain", "inspect", "repo"},
			events:      []app.Event{{Type: app.EventOutputDelta, Text: "1\n2\n3\n4\n5\n6\n7\n"}},
			contains:    []string{"7\n"},
			notContains: []string{"truncated"},
		},
		{
			name: "resets streamed output limit for each tool execution",
			cfg:  taskLiveOutputLimitConfig(2),
			args: []string{"--plain", "inspect", "repo"},
			events: []app.Event{
				{Type: app.EventTaskStatus, Status: string(agent.TaskStatusRunningTool), Text: "Running unix...", ToolName: tools.ToolNameUnix},
				{Type: app.EventOutputDelta, Text: "1\n2\n3\n", ToolName: tools.ToolNameUnix},
				{Type: app.EventTaskStatus, Status: string(agent.TaskStatusRunningTool), Text: "Running python...", ToolName: tools.ToolNamePython},
				{Type: app.EventOutputDelta, Text: "a\nb\nc\n", ToolName: tools.ToolNamePython},
			},
			contains: []string{
				"1\n2\n",
				"tool output truncated: 2 out of 3 lines",
				"a\nb\n",
				"done",
			},
			notContains: []string{"3\n", "c\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			if cfg == nil {
				cfg = config.NewDefaultConfig()
			}
			output := runTaskCommandWithConfigAndEvents(t, cfg, tt.args, func(req app.TaskRequest) []app.Event {
				events := append([]app.Event{}, tt.events...)
				events = append(events, app.Event{Type: app.EventCompleted, FinalOutput: "done", Status: req.Message})
				return events
			})

			if tt.want != "" || len(tt.contains) == 0 && len(tt.notContains) == 0 {
				assert.Equal(t, tt.want, output)
			}
			for _, want := range tt.contains {
				assert.Contains(t, output, want)
			}
			for _, unwanted := range tt.notContains {
				assert.NotContains(t, output, unwanted)
			}
		})
	}
}

func TestTaskLiveOutputLimiter(t *testing.T) {
	sourceFileOutput := strings.Join([]string{
		"def create_bingo_board(entries):",
		"    if len(entries) != 25:",
		"        raise ValueError(\"The list must contain exactly 25 entries.\")",
		"",
		"    # Create a 5x5 bingo board",
		"    bingo_board = [entries[i : i + 5] for i in range(0, 25, 5)]",
		"    return bingo_board",
	}, "\n") + "\n"

	tests := []struct {
		name         string
		maxLines     int
		maxLineChars int
		chunks       []string
		wantChunks   []string
		contains     []string
		notContains  []string
	}{
		{
			name:         "limits long single line",
			maxLines:     2,
			maxLineChars: 4,
			chunks:       []string{"abcdefghijkl", "more"},
			wantChunks:   []string{"abcdefgh\n[tool output truncated: 2 lines shown]\n", ""},
		},
		{
			name:         "spans chunks",
			maxLines:     2,
			maxLineChars: 120,
			chunks:       []string{"1\n", "2\n3\n", "4\n"},
			wantChunks:   []string{"1\n", "2\n[tool output truncated: 2 out of 3 lines]\n", ""},
		},
		{
			name:         "shows configured lines for source file output",
			maxLines:     6,
			maxLineChars: 120,
			chunks:       []string{sourceFileOutput},
			contains: []string{
				"def create_bingo_board(entries):\n",
				"    if len(entries) != 25:\n",
				"        raise ValueError",
				"    # Create a 5x5 bingo board\n",
				"    bingo_board = [entries[i : i + 5] for i in range(0, 25, 5)]\n",
				"tool output truncated: 6 out of 7 lines",
			},
			notContains: []string{"    return bingo_board\n"},
		},
		{
			name:         "uses character fallback only before newline",
			maxLines:     2,
			maxLineChars: 4,
			chunks:       []string{"1234567\n", "abcdefghijk\nthird\n"},
			wantChunks:   []string{"1234567\n", "abcdefghijk\n[tool output truncated: 2 out of 3 lines]\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := newTaskLiveOutputLimiter(tt.maxLines, tt.maxLineChars)
			var output strings.Builder
			for i, chunk := range tt.chunks {
				got := limiter.Filter(chunk)
				if len(tt.wantChunks) > 0 {
					assert.Equal(t, tt.wantChunks[i], got)
				}
				output.WriteString(got)
			}
			for _, want := range tt.contains {
				assert.Contains(t, output.String(), want)
			}
			for _, unwanted := range tt.notContains {
				assert.NotContains(t, output.String(), unwanted)
			}
		})
	}
}

func TestTaskCommandProgressOutput(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		contains    []string
		notContains []string
	}{
		{
			name:     "prints progress when always enabled",
			args:     []string{"--progress=always", "--plain", "inspect", "repo"},
			contains: []string{"Thinking...\n", "file_search: scanning\n", "done"},
		},
		{
			name:        "suppresses progress when never enabled",
			args:        []string{"--progress=never", "--plain", "inspect", "repo"},
			contains:    []string{"done"},
			notContains: []string{"Thinking...", "file_search: scanning"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := runTaskCommandWithEvents(t, tt.args, func(req app.TaskRequest) []app.Event {
				return []app.Event{
					{Type: app.EventTaskStatus, Text: "Thinking..."},
					{Type: app.EventToolProgress, ToolName: "file_search", Text: "scanning"},
					{Type: app.EventCompleted, FinalOutput: "done", Status: req.Message},
				}
			})

			for _, want := range tt.contains {
				assert.Contains(t, output, want)
			}
			for _, unwanted := range tt.notContains {
				assert.NotContains(t, output, unwanted)
			}
		})
	}

	t.Run("rejects invalid progress mode", func(t *testing.T) {
		originalNewService := newService
		defer func() { newService = originalNewService }()

		newService = func() app.Service {
			return &fakeTaskService{events: func(_ context.Context, req app.TaskRequest) (<-chan app.Event, error) {
				ch := make(chan app.Event)
				go func() {
					defer close(ch)
					ch <- app.Event{Type: app.EventCompleted, FinalOutput: "done", Status: req.Message}
				}()
				return ch, nil
			}}
		}

		cmd := NewTaskCommand(config.NewDefaultConfig())
		cmd.SetIn(bytes.NewBufferString(""))
		output := &bytes.Buffer{}
		cmd.SetOut(output)
		cmd.SetErr(output)
		cmd.Flags().String("device", "", "")
		cmd.SetArgs([]string{"--progress=nope", "inspect", "repo"})

		err := cmd.ExecuteContext(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid --progress value")
	})
}

func TestTaskProgressPrinterOutput(t *testing.T) {
	tests := []struct {
		name        string
		config      taskProgressConfig
		messages    []string
		want        string
		contains    []string
		notContains []string
	}{
		{
			name:        "interactive replaces status line",
			config:      taskProgressConfig{enabled: true, interactive: true},
			messages:    []string{"Thinking...", "Running file_search..."},
			contains:    []string{"\r\x1b[2K| Thinking...", "\r\x1b[2K| Running file_search...", "\r\x1b[2K"},
			notContains: []string{"Thinking...\n"},
		},
		{
			name:     "non-interactive prints unique progress lines",
			config:   taskProgressConfig{enabled: true},
			messages: []string{"Thinking...", "Thinking...", "Running file_search..."},
			want:     "Thinking...\nRunning file_search...\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := &bytes.Buffer{}
			progress := newTaskProgressPrinter(output, tt.config)
			for _, message := range tt.messages {
				progress.Print(message)
			}
			progress.Clear()

			text := output.String()
			if tt.want != "" {
				assert.Equal(t, tt.want, text)
			}
			for _, want := range tt.contains {
				assert.Contains(t, text, want)
			}
			for _, unwanted := range tt.notContains {
				assert.NotContains(t, text, unwanted)
			}
		})
	}
}

func taskLiveOutputLimitConfig(limit int) config.Config {
	cfg := config.NewDefaultConfig()
	cfg.TaskLiveOutputLimit = &limit
	return cfg
}

func runTaskCommandWithEvents(t *testing.T, args []string, events func(app.TaskRequest) []app.Event) string {
	return runTaskCommandWithConfigAndEvents(t, config.NewDefaultConfig(), args, events)
}

func runTaskCommandWithConfigAndEvents(t *testing.T, cfg config.Config, args []string, events func(app.TaskRequest) []app.Event) string {
	t.Helper()
	originalNewService := newService
	defer func() { newService = originalNewService }()

	newService = func() app.Service {
		return &fakeTaskService{events: func(_ context.Context, req app.TaskRequest) (<-chan app.Event, error) {
			ch := make(chan app.Event)
			go func() {
				defer close(ch)
				for _, event := range events(req) {
					ch <- event
				}
			}()
			return ch, nil
		}}
	}

	cmd := NewTaskCommand(cfg)
	output := &bytes.Buffer{}
	cmd.SetIn(bytes.NewBufferString(""))
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.Flags().String("device", "", "")
	cmd.SetArgs(args)

	require.NoError(t, cmd.ExecuteContext(context.Background()))
	return output.String()
}

// runTaskCommandCapture runs the task command with the given config and args,
// returning the TaskRequest the service received.
func runTaskCommandCapture(t *testing.T, cfg config.Config, args ...string) app.TaskRequest {
	t.Helper()
	originalNewService := newService
	defer func() { newService = originalNewService }()

	var captured app.TaskRequest
	newService = func() app.Service {
		return &fakeTaskService{events: func(_ context.Context, req app.TaskRequest) (<-chan app.Event, error) {
			captured = req
			ch := make(chan app.Event)
			go func() {
				defer close(ch)
				ch <- app.Event{Type: app.EventCompleted, FinalOutput: "done", Status: req.Message}
			}()
			return ch, nil
		}}
	}

	cmd := NewTaskCommand(cfg)
	cmd.SetIn(bytes.NewBufferString(""))
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.Flags().String("device", "", "")
	cmd.SetArgs(args)

	require.NoError(t, cmd.ExecuteContext(context.Background()))
	return captured
}

func TestTaskCommandResolvesTimeout(t *testing.T) {
	t.Run("defaults to unlimited (zero) when neither flag nor config set", func(t *testing.T) {
		req := runTaskCommandCapture(t, config.NewDefaultConfig(), "do", "something")
		assert.Equal(t, time.Duration(0), req.Timeout)
	})

	t.Run("uses --timeout flag when provided", func(t *testing.T) {
		req := runTaskCommandCapture(t, config.NewDefaultConfig(), "--timeout", "30s", "do", "something")
		assert.Equal(t, 30*time.Second, req.Timeout)
	})

	t.Run("falls back to config when flag absent", func(t *testing.T) {
		cfg := config.NewDefaultConfig()
		cfg.TaskTimeout = "5m"
		req := runTaskCommandCapture(t, cfg, "do", "something")
		assert.Equal(t, 5*time.Minute, req.Timeout)
	})

	t.Run("flag overrides config", func(t *testing.T) {
		cfg := config.NewDefaultConfig()
		cfg.TaskTimeout = "5m"
		req := runTaskCommandCapture(t, cfg, "--timeout", "1m", "do", "something")
		assert.Equal(t, time.Minute, req.Timeout)
	})

	t.Run("explicit --timeout 0 overrides config back to unlimited", func(t *testing.T) {
		cfg := config.NewDefaultConfig()
		cfg.TaskTimeout = "5m"
		req := runTaskCommandCapture(t, cfg, "--timeout", "0", "do", "something")
		assert.Equal(t, time.Duration(0), req.Timeout)
	})
}

func TestTaskCommandPassesAutoApprove(t *testing.T) {
	req := runTaskCommandCapture(t, config.NewDefaultConfig(), "--auto-approve", "do", "something")
	assert.True(t, req.AutoApprove)
}
