package routines

import (
	"os"
	"path/filepath"
	"strings"
)

// Environment overrides let tests redirect routine storage away from the real
// user directories, mirroring TERMINAL_AGENT_SESSIONS_DIR for session logs.
const (
	// DefinitionsFileEnv overrides the path to the routines definitions file.
	DefinitionsFileEnv = "TERMINAL_AGENT_ROUTINES_FILE"
	// DataDirEnv overrides the directory holding routine run state and logs.
	DataDirEnv = "TERMINAL_AGENT_ROUTINES_DIR"
)

// DefinitionsPath is the routines definitions file, alongside config.json in the
// user config directory.
func DefinitionsPath() string {
	if override := strings.TrimSpace(os.Getenv(DefinitionsFileEnv)); override != "" {
		return override
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "terminal-agent", "routines.json")
}

// DataDir is the dedicated directory for routine run state and logs, separate
// from the shared session-log directory.
func DataDir() string {
	if override := strings.TrimSpace(os.Getenv(DataDirEnv)); override != "" {
		return override
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "share", "terminal-agent", "routines")
}

// StatePath is the run-status index file inside the routines data dir.
func StatePath() string {
	return filepath.Join(DataDir(), "state.json")
}

// LogDir is the per-routine directory holding that routine's run logs and result
// summaries.
func LogDir(id string) string {
	return filepath.Join(DataDir(), id, "logs")
}
