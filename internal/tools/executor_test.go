package tools

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBashExecutorDoesNotPrintWorkingDirectory(t *testing.T) {
	tempDir := t.TempDir()
	executor := &BashExecutor{workDir: tempDir}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() {
		os.Stdout = stdout
	}()

	output, execErr := executor.Exec("pwd")
	require.NoError(t, execErr)
	assert.Equal(t, filepath.Clean(tempDir), output)

	require.NoError(t, w.Close())
	printed, readErr := io.ReadAll(r)
	require.NoError(t, readErr)

	assert.Contains(t, string(printed), "Executing Unix command: pwd")
	assert.NotContains(t, string(printed), "Working directory:")
	assert.False(t, strings.Contains(output, "Working directory:"))
}
