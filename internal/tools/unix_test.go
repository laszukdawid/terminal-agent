package tools

import (
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
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
			prompt:   "{ \"code\": \"\"}",
			err:      "invalid unix command",
			expected: "",
		},
		{
			name:     "Supported command",
			prompt:   `ls`,
			err:      "",
			expected: "exec: ls",
		},
		{
			name:     "No files deletion for now",
			prompt:   `rm *`,
			err:      "invalid unix command",
			expected: "",
		},
		{
			name:     "Supported command but contains 'sudo'",
			prompt:   `sudo rm -rf /`,
			err:      "command requires sudo which is not allowed",
			expected: "",
		},
		{
			name:     "Hidden sudo command",
			prompt:   `true && sudo rm -rf /`,
			err:      "command requires sudo which is not allowed",
			expected: "",
		},
		{
			name:     "Unsupported command in code",
			prompt:   `unsupported some garbage`,
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

func TestUnixToolHelpText(t *testing.T) {
	// Create a new UnixTool instance
	unixTool := NewUnixTool(nil)

	// Call the HelpText method
	helpText := unixTool.HelpText()

	// Verify that the help text matches the expected constant
	assert.Equal(t, unixToolHelp, helpText)

	// Verify that the help text contains expected sections
	assert.Contains(t, helpText, "UNIX TOOL HELP")
	assert.Contains(t, helpText, "NAME")
	assert.Contains(t, helpText, "DESCRIPTION")
	assert.Contains(t, helpText, "USAGE")
	assert.Contains(t, helpText, "INPUT SCHEMA")
	assert.Contains(t, helpText, "OUTPUT")
	assert.Contains(t, helpText, "SECURITY NOTES")
}
