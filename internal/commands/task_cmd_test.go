package commands

import (
	"bytes"
	"context"
	"testing"

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

func (s *fakeTaskService) Task(context.Context, app.TaskRequest) (app.TaskResult, error) {
	panic("unexpected Task call")
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
						Action: `unix("git status")`,
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
	input := bytes.NewBufferString("yes!\ninternal\n")
	output := &bytes.Buffer{}
	cmd.SetIn(input)
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.Flags().String("device", "", "")
	cmd.SetArgs([]string{"inspect", "repo"})

	err := cmd.ExecuteContext(context.Background())
	require.NoError(t, err)
	assert.Equal(t, app.TaskConfirmationResponse{Allowed: true, Remember: true}, confirmation)
	assert.Equal(t, "internal", clarification)
	assert.Contains(t, output.String(), "Execute the following action?")
	assert.Contains(t, output.String(), "Need clarification: Which directory?")
	assert.Contains(t, output.String(), "done")
}
