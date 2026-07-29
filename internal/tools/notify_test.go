package tools

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeNotifyExecutor struct {
	lookupErr error
	runErr    error
	output    string
	name      string
	args      []string
}

func (e *fakeNotifyExecutor) LookPath(string) (string, error) {
	return notifyCommand, e.lookupErr
}

func (e *fakeNotifyExecutor) Run(name string, args ...string) (string, error) {
	e.name = name
	e.args = args
	return e.output, e.runErr
}

func TestNotifyUserToolRunSchema(t *testing.T) {
	tests := []struct {
		name      string
		input     map[string]any
		lookupErr error
		runErr    error
		output    string
		want      string
		wantErr   string
		wantArgs  []string
	}{
		{
			name:     "uses default title",
			input:    map[string]any{"message": "CPU usage crossed 50%"},
			want:     notifySentMessage,
			wantArgs: []string{"--title", defaultNotifyTitle, "--message", "CPU usage crossed 50%"},
		},
		{
			name:  "passes custom title and urgency",
			input: map[string]any{"title": "Build", "message": "completed", "urgency": notifyUrgencyCritical},
			output: "delivered",
			want:   "delivered",
			wantArgs: []string{
				"--title", "Build", "--message", "completed", "--urgency", notifyUrgencyCritical,
			},
		},
		{
			name:      "reports missing command",
			input:     map[string]any{"message": "completed"},
			lookupErr: exec.ErrNotFound,
			wantErr:   notifyCommandMissingMessage,
		},
		{
			name:    "reports command failure",
			input:   map[string]any{"message": "completed"},
			runErr:  errors.New("delivery failed"),
			wantErr: "failed to send notification",
		},
		{
			name:    "requires message",
			input:   map[string]any{},
			wantErr: "notification message is required",
		},
		{
			name:    "rejects unsupported urgency",
			input:   map[string]any{"message": "completed", "urgency": "urgent"},
			wantErr: "notification urgency must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &fakeNotifyExecutor{lookupErr: tt.lookupErr, runErr: tt.runErr, output: tt.output}
			tool := newNotifyUserTool(executor)

			got, err := tool.RunSchema(tt.input)

			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Empty(t, got)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, notifyCommand, executor.name)
			assert.Equal(t, tt.wantArgs, executor.args)
		})
	}
}

func TestNotifyUserToolPermissionCategory(t *testing.T) {
	assert.Equal(t, PermissionExecute, NewNotifyUserTool().PermissionCategory())
}
