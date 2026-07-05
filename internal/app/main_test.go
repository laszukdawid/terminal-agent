package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain redirects session logs and the task memory database to a throwaway
// directory so tests never read or write the real ~/.local/share/terminal-agent data.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "terminal-agent-sessions")
	if err == nil {
		_ = os.Setenv(SessionDirEnv, filepath.Join(dir, "sessions"))
		_ = os.Setenv(TaskMemoryPathEnv, filepath.Join(dir, "task_memory.db"))
	}
	code := m.Run()
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}
