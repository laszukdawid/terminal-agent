package tools

import (
	"io"
	"os"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type mockBashExecutor struct {
}

func (m *mockBashExecutor) Exec(code string) (string, error) {
	return "exec: " + code, nil
}

func TestMain(m *testing.M) {
	loglevel := zap.DebugLevel.String()
	logger, _ := utils.InitLogger(&loglevel)
	defer logger.Sync()

	m.Run()
}

func TestUnixToolsRun(t *testing.T) {
	mockExecutor := &mockBashExecutor{}
	tools := NewUnixTool(mockExecutor)

	tests := []struct {
		name     string
		prompt   string
		err      string
		expected string
	}{
		{
			name:     "Empty code",
			prompt:   "",
			err:      "no Unix command found",
			expected: "",
		},
		{
			name:     "Supported command",
			prompt:   `ls`,
			err:      "",
			expected: "exec: ls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			m := map[string]any{
				"command": tt.prompt,
			}
			out, err := tools.RunSchema(m)

			if tt.err != "" {
				assert.Error(t, err)
				assert.Containsf(t, err.Error(), tt.err, "Error message should contain %s", tt.err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, out)
			}
		})
	}

}

func TestUnixToolDoesNotPrintExecutionInternals(t *testing.T) {
	mockExecutor := &mockBashExecutor{}
	tool := NewUnixTool(mockExecutor)

	stdout := os.Stdout
	r, w, err := os.Pipe()
	assert.NoError(t, err)
	os.Stdout = w
	defer func() {
		os.Stdout = stdout
	}()

	out, runErr := tool.RunSchema(map[string]any{"command": "ls"})
	assert.NoError(t, runErr)
	assert.Equal(t, "exec: ls", out)

	assert.NoError(t, w.Close())
	printed, readErr := io.ReadAll(r)
	assert.NoError(t, readErr)
	assert.Empty(t, string(printed))
}
