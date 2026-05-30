package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatTaskTimeout(t *testing.T) {
	assert.Equal(t, "unlimited", formatTaskTimeout(0))
	assert.Equal(t, "15m0s", formatTaskTimeout(15*time.Minute))
	assert.Equal(t, "1m30s", formatTaskTimeout(90*time.Second))
}

func TestResolveTaskRootDirUsesExplicitRequestWorkingDir(t *testing.T) {
	requestDir := t.TempDir()
	configuredDir := t.TempDir()

	rootDir, err := resolveTaskRootDir(TaskRequest{
		WorkingDir: requestDir,
		Config:     config.WithWorkingDir(config.NewDefaultConfig(), configuredDir),
	})

	require.NoError(t, err)
	assert.Equal(t, requestDir, rootDir)
}

func TestResolveTaskRootDirFallsBackToConfiguredWorkingDir(t *testing.T) {
	configuredDir := t.TempDir()

	rootDir, err := resolveTaskRootDir(TaskRequest{
		Config: config.WithWorkingDir(config.NewDefaultConfig(), configuredDir),
	})

	require.NoError(t, err)
	assert.Equal(t, configuredDir, rootDir)
}

func TestResolveTaskRootDirFallsBackToProcessCwd(t *testing.T) {
	originalWD, err := os.Getwd()
	require.NoError(t, err)
	tempDir := t.TempDir()
	require.NoError(t, os.Chdir(tempDir))
	defer os.Chdir(originalWD)

	rootDir, err := resolveTaskRootDir(TaskRequest{Config: config.NewDefaultConfig()})

	require.NoError(t, err)
	// os.Getwd resolves symlinks (e.g. macOS /var -> /private/var), so compare against
	// the resolved temp dir rather than the raw t.TempDir() path.
	expected, err := filepath.EvalSymlinks(tempDir)
	require.NoError(t, err)
	assert.Equal(t, expected, rootDir)
}
