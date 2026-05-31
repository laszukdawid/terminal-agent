package tools

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type mockBashExecutor struct {
}

type testOutputWriter func([]byte) (int, error)

func (w testOutputWriter) Write(p []byte) (int, error) {
	return w(p)
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

func TestBashExecutor(t *testing.T) {
	t.Run("streams output before command completes", func(t *testing.T) {
		chunks := make(chan string, 4)
		executor := &BashExecutor{output: testOutputWriter(func(p []byte) (int, error) {
			chunks <- string(p)
			return len(p), nil
		})}
		done := make(chan struct {
			output string
			err    error
		}, 1)

		go func() {
			output, err := executor.Exec(`printf first; sleep 0.2; printf second`)
			done <- struct {
				output string
				err    error
			}{output: output, err: err}
		}()

		select {
		case chunk := <-chunks:
			assert.Equal(t, "first", chunk)
		case <-done:
			t.Fatal("command completed before streaming first output chunk")
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for streamed output")
		}

		select {
		case result := <-done:
			assert.NoError(t, result.err)
			assert.Equal(t, "firstsecond", result.output)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for command completion")
		}
	})
}
