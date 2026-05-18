package config

import "os"

type workingDirConfig struct {
	Config
	workingDir string
}

func WithWorkingDir(cfg Config, workingDir string) Config {
	if cfg == nil {
		return nil
	}
	if workingDir == "" {
		if wd, err := os.Getwd(); err == nil {
			workingDir = wd
		}
	}
	if workingDir == "" {
		return cfg
	}
	return &workingDirConfig{Config: cfg, workingDir: workingDir}
}

func (c *workingDirConfig) GetWorkingDir() string {
	if c.workingDir != "" {
		return c.workingDir
	}
	return c.Config.GetWorkingDir()
}
