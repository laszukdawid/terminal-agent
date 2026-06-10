package tools

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBashExecutorDoesNotPrintExecutionInternals(t *testing.T) {
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

	assert.Empty(t, string(printed))
	assert.False(t, strings.Contains(output, "Working directory:"))
}

func TestBashExecutorExecContextCancelsLongRunningCommand(t *testing.T) {
	executor := &BashExecutor{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := executor.ExecContext(ctx, "sleep 5")
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, 2*time.Second)
}

func TestBashExecutorExecContextWithOptionsTimeoutReturnsResult(t *testing.T) {
	executor := &BashExecutor{}

	start := time.Now()
	result, err := executor.ExecContextWithOptions(context.Background(), `printf start; sleep 5`, ProcessOptions{Timeout: 100 * time.Millisecond})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.True(t, result.TimedOut)
	assert.Equal(t, "start", result.Output)
	assert.Less(t, elapsed, 2*time.Second)
}

func TestBashExecutorExecContextWithOptionsParentCancellationWins(t *testing.T) {
	executor := &BashExecutor{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := executor.ExecContextWithOptions(ctx, `printf start; sleep 5`, ProcessOptions{Timeout: time.Hour})

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestBashExecutorExecContextWithOptionsMaxBytesCapsCaptureOnly(t *testing.T) {
	var streamed strings.Builder
	executor := &BashExecutor{output: testOutputWriter(func(p []byte) (int, error) {
		streamed.Write(p)
		return len(p), nil
	})}

	result, err := executor.ExecContextWithOptions(context.Background(), `printf abcdef`, ProcessOptions{MaxBytes: 3})

	require.NoError(t, err)
	assert.Equal(t, "abc", result.Output)
	assert.True(t, result.Truncated)
	assert.Equal(t, 3, result.CapturedBytes)
	assert.Equal(t, "abcdef", streamed.String())
}

func TestBashExecutorFailureIncludesCapturedOutput(t *testing.T) {
	executor := &BashExecutor{}

	_, err := executor.ExecContextWithOptions(context.Background(), `printf abcdef; exit 7`, ProcessOptions{MaxBytes: 3})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Output: abc")
	assert.NotContains(t, err.Error(), "Process output exceeded capture limit")
}
