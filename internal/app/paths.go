package app

import (
	"os"
	"path/filepath"
)

var (
	memoryDir  = filepath.Join(os.Getenv("HOME"), ".local", "share", "terminal-agent")
	memoryFile = "memory.jsonl"
	logDir     = filepath.Join(os.Getenv("HOME"), ".local", "share", "terminal-agent")
	logFile    = "query_log.jsonl"
)

func MemoryPath() string {
	return filepath.Join(memoryDir, memoryFile)
}

func LogPath() string {
	return filepath.Join(logDir, logFile)
}
