//go:build integration
// +build integration

package tools

import (
	"os"
	"testing"

	_ "github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/stretchr/testify/assert"
)

func TestUnixToolsRunIntegration(t *testing.T) {
	// Workdir is based on env var TEST_INTEG_DIR
	workDir := os.Getenv("TEST_INTEG_DIR")

	connector := &llmConnectorMock{}
	bashExecutor := &BashExecutor{
		confirmPrompt: false,
		workDir:       workDir,
	}
	tools := NewUnixTool(connector, bashExecutor)

	tests := []struct {
		name     string
		prompt   string
		err      string
		expected string
	}{
		{
			name:     "List files",
			prompt:   `{"code": "ls"}`,
			err:      "",
			expected: "bash_script.sh\ndir1\ndir2\ntext_file.txt",
		}, {
			name:     "Print working directory",
			prompt:   `{"code": "pwd"}`,
			err:      "",
			expected: workDir,
		}, {
			name:     "List files in a directory",
			prompt:   `{"code": "ls dir1"}`,
			err:      "",
			expected: "t1",
		}, {
			name:     "Read file",
			prompt:   `{ "code": "cat text_file.txt"}`,
			err:      "",
			expected: "Here be by some text purely for testing",
		}, {
			name:     "List files in a non existing directory",
			prompt:   `{"code": "ls not-existing-dir"}`,
			err:      "failed to execute Unix command: exit status",
			expected: "",
		}, {
			name:     "sudo something not allowed",
			prompt:   `{ "code": "sudo chmod 777"}`,
			err:      "command requires sudo which is not allowed",
			expected: "",
		}, {
			name:     "Removing files is not supported",
			prompt:   `{ "code": "rm *"}`,
			err:      "invalid unix command",
			expected: "",
		}, {
			name:     "Never allow deleting root",
			prompt:   `{"code": "sudo rm -rf /"}`,
			err:      "not allowed",
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
