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
	t.Run("pipefail catches early pipeline failures", func(t *testing.T) {
		executor := &BashExecutor{}
		_, err := executor.Exec("false | true")
		assert.Error(t, err, "pipefail should propagate failure from early pipeline stage")
	})

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

func TestUnixToolTimeoutReturnsCapturedOutput(t *testing.T) {
	tool := NewUnixTool(nil)

	out, err := tool.RunSchema(map[string]any{
		"command": "printf start; sleep 5",
		"timeout": "100ms",
	})

	assert.NoError(t, err)
	assert.Equal(t, "start", out)
}

func TestUnixToolMaxBytesCapsCapturedOutput(t *testing.T) {
	tool := NewUnixTool(nil)

	out, err := tool.RunSchema(map[string]any{
		"command":   "printf abcdef",
		"max_bytes": 3,
	})

	assert.NoError(t, err)
	assert.Equal(t, "abc", out)
}

func TestUnixToolZeroTimeoutAndMaxBytesMeanUnlimited(t *testing.T) {
	tool := NewUnixTool(nil)

	out, err := tool.RunSchema(map[string]any{
		"command":   "printf abcdef",
		"timeout":   "0",
		"max_bytes": 0,
	})

	assert.NoError(t, err)
	assert.Equal(t, "abcdef", out)
}

func TestUnixToolRejectsInvalidProcessOptions(t *testing.T) {
	tool := NewUnixTool(nil)

	_, err := tool.RunSchema(map[string]any{"command": "true", "timeout": "not-a-duration"})
	assert.ErrorContains(t, err, "invalid timeout")

	_, err = tool.RunSchema(map[string]any{"command": "true", "max_bytes": 1.5})
	assert.ErrorContains(t, err, "max_bytes must be an integer")
}
