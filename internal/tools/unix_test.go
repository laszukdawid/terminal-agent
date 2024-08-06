package tools

import (
	"testing"

	_ "github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/stretchr/testify/assert"
)

type llmConnectorMock struct {
}

func (m *llmConnectorMock) Query(userPrompt *string, sysPrompt *string) (string, error) {
	return "prompt: " + *userPrompt, nil
}

type mockBashExecutor struct {
}

func (m *mockBashExecutor) Exec(code string) (string, error) {
	return "exec: " + code, nil
}

func TestMain(m *testing.M) {
	logger := utils.InitLogger()
	defer logger.Sync()

	m.Run()
}

func TestUnixToolsRun(t *testing.T) {
	connector := &llmConnectorMock{}
	mockExecutor := &mockBashExecutor{}
	tools := NewUnixTool(connector, mockExecutor)

	tests := []struct {
		name     string
		prompt   string
		err      string
		expected string
	}{
		{
			name:     "Valid command",
			prompt:   "garbage",
			err:      "no code object: no JSON objects found in the input",
			expected: "",
		},
		{
			name:     "Error from connector",
			prompt:   `Something { "code": but never finished`,
			err:      "no code object",
			expected: "",
		},
		{
			name:     "Empty code",
			prompt:   "{ \"code\": \"\"}",
			err:      "no code object found in the input",
			expected: "",
		},
		{
			name:     "Supported command",
			prompt:   `{"code": "ls"}`,
			err:      "",
			expected: "exec: ls",
		},
		{
			name:     "No files deletion for now",
			prompt:   `{"code": "rm *"}`,
			err:      "invalid unix command",
			expected: "",
		},
		{
			name:     "Supported command but contains 'sudo'",
			prompt:   `{"code": "sudo rm -rf /"}`,
			err:      "command requires sudo which is not allowed",
			expected: "",
		},
		{
			name:     "Hidden sudo command",
			prompt:   `{"code": "true && sudo rm -rf /"}`,
			err:      "command requires sudo which is not allowed",
			expected: "",
		},
		{
			name:     "Unsupported command in code",
			prompt:   `{"code": "unsupported some garbage"}`,
			err:      "invalid unix command",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			out, err := tools.Run(&tt.prompt)

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
