package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain redirects session logs to a throwaway directory so the always-on execution
// logging never writes into the real ~/.local/share/terminal-agent/sessions during tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "terminal-agent-sessions")
	if err == nil {
		_ = os.Setenv(SessionDirEnv, filepath.Join(dir, "sessions"))
	}
	code := m.Run()
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}
