//go:build integration
// +build integration

package tools

import (
	"testing"

	_ "github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/stretchr/testify/assert"
)

func TestUnixToolsRunIntegration(t *testing.T) {
	connector := &llmConnectorMock{}
	bashExecutor := &BashExecutor{
		confirmPrompt: false,
		workDir:       "/agent/test",
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
			prompt:   "<code>ls</code>",
			err:      "",
			expected: "bash_script.sh\ndir1\ndir2\ntext_file.txt",
		}, {
			name:     "Print working directory",
			prompt:   "<code>pwd</code>",
			err:      "",
			expected: "/agent/test",
		}, {
			name:     "List files in a directory",
			prompt:   "<code>ls dir1</code>",
			err:      "",
			expected: "t1",
		}, {
			name:     "Read file",
			prompt:   "<code>cat text_file.txt</code>",
			err:      "",
			expected: "Here be by some text purely for testing",
		}, {
			name:     "List files in a non existing directory",
			prompt:   "<code>ls not-existing-dir/</code>",
			err:      "failed to execute Unix command: exit status 1",
			expected: "",
		}, {
			name:     "sudo something not allowed",
			prompt:   "<code>sudo chmod 777 *</code>",
			err:      "command requires sudo which is not allowed",
			expected: "",
		}, {
			name:     "Removing files is not supported",
			prompt:   "<code>rm *</code>",
			err:      "invalid Unix command found in the response",
			expected: "",
		}, {
			name:     "Never allow deleting root",
			prompt:   "<code>sudo rm -rf /</code>",
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
