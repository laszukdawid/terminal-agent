package commands

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildContextFromTerminalRequiresPlugin(t *testing.T) {
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	_, err := buildContextFromTerminal(3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), bashReaderInstallHint)
}

func TestBuildContextFromTerminalReturnsLatestThreeEntries(t *testing.T) {
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	result, err := installBashReaderPlugin(tempDir)
	require.NoError(t, err)
	require.NotEmpty(t, result.ScriptPath)

	contextDir := getTerminalContextDir(tempDir)
	indexPath := filepath.Join(contextDir, "index.log")

	output1 := filepath.Join(contextDir, "sessions", "1.log")
	output2 := filepath.Join(contextDir, "sessions", "2.log")
	output3 := filepath.Join(contextDir, "sessions", "3.log")
	output4 := filepath.Join(contextDir, "sessions", "4.log")

	require.NoError(t, os.WriteFile(output1, []byte("first output"), 0644))
	require.NoError(t, os.WriteFile(output2, []byte("second output"), 0644))
	require.NoError(t, os.WriteFile(output3, []byte("third output"), 0644))
	require.NoError(t, os.WriteFile(output4, []byte("fourth output"), 0644))

	line1 := makeTerminalContextLine(1, 0, output1, "echo first")
	line2 := makeTerminalContextLine(2, 0, output2, "date")
	line3 := makeTerminalContextLine(3, 1, output3, "rm dir1/dir2/file.md")
	line4 := makeTerminalContextLine(4, 0, output4, "ls -la")

	indexContent := fmt.Sprintf("%s\n%s\n%s\n%s\n", line1, line2, line3, line4)
	require.NoError(t, os.WriteFile(indexPath, []byte(indexContent), 0644))

	context, err := buildContextFromTerminal(3)
	require.NoError(t, err)

	assert.Contains(t, context, "<context>")
	assert.NotContains(t, context, "echo first")
	assert.Contains(t, context, "$ date")
	assert.Contains(t, context, "$ rm dir1/dir2/file.md")
	assert.Contains(t, context, "$ ls -la")
	assert.Contains(t, context, "exit_code: 1")
	assert.Contains(t, context, "third output")
	assert.Contains(t, context, "fourth output")
	assert.Contains(t, context, "</context>")
}

func makeTerminalContextLine(timestamp int64, exitCode int, outputPath, command string) string {
	return fmt.Sprintf("%d\t%d\t%s\t%s", timestamp, exitCode, outputPath, base64.StdEncoding.EncodeToString([]byte(command)))
}
