package app

import (
	"os"
	"path/filepath"
	"strings"
)

var (
	memoryDir  = filepath.Join(os.Getenv("HOME"), ".local", "share", "terminal-agent")
	memoryFile = "memory.jsonl"
	logDir     = filepath.Join(os.Getenv("HOME"), ".local", "share", "terminal-agent")
	logFile    = "query_log.jsonl"
	sessionDir = filepath.Join(os.Getenv("HOME"), ".local", "share", "terminal-agent", "sessions")
)

func MemoryPath() string {
	return filepath.Join(memoryDir, memoryFile)
}

func LogPath() string {
	return filepath.Join(logDir, logFile)
}

// SessionDirEnv overrides the directory where per-run execution logs are written.
// Primarily used by tests to avoid writing into the real user data directory.
const SessionDirEnv = "TERMINAL_AGENT_SESSIONS_DIR"

// SessionDir is the directory where per-run execution logs are written.
func SessionDir() string {
	if override := strings.TrimSpace(os.Getenv(SessionDirEnv)); override != "" {
		return override
	}
	return sessionDir
}
