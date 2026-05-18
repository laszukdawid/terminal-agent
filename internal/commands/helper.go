package commands

import "github.com/laszukdawid/terminal-agent/internal/config"

func resolveExecutionConfig(cfg config.Config) config.Config {
	return config.WithWorkingDir(cfg, "")
}
