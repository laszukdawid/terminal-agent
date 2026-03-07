//go:build integration
// +build integration

package tools

import (
	"os"
	"testing"

	// _ "github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/stretchr/testify/assert"
)

func TestUnixToolsRunIntegration(t *testing.T) {
	// Workdir is based on env var TEST_INTEG_DIR
	workDir := os.Getenv("TEST_INTEG_DIR")

	bashExecutor := &BashExecutor{
		workDir: workDir,
	}
	tool := NewUnixTool(bashExecutor)

	tests := []struct {
		name     string
		prompt   string
		err      string
		expected string
	}{
		{
			name:     "List files",
			prompt:   `ls`,
			err:      "",
			expected: "bash_script.sh\ndir1\ndir2\ntext_file.txt",
		}, {
			name:     "Print working directory",
			prompt:   `pwd`,
			err:      "",
			expected: workDir,
		}, {
			name:     "List files in a directory",
			prompt:   `ls dir1`,
			err:      "",
			expected: "t1",
		}, {
			name:     "Read file",
			prompt:   `cat text_file.txt`,
			err:      "",
			expected: "Here be by some text purely for testing",
		}, {
			name:     "List files in a non existing directory",
			prompt:   `ls not-existing-dir`,
			err:      "ls: cannot access 'not-existing-dir': No such file or directory",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			out, err := tool.Run(&tt.prompt)

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
