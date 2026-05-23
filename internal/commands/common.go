package commands

import (
	"fmt"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/spf13/pflag"
)

func resolveDevice(flags *pflag.FlagSet, cfg config.Config) (string, error) {
	device, err := flags.GetString("device")
	if err != nil {
		return "", err
	}

	if device == "" {
		if cfg == nil {
			return "auto", nil
		}
		return cfg.GetDevice(), nil
	}

	switch device {
	case "auto", "cpu", "gpu":
		return device, nil
	default:
		return "", fmt.Errorf("invalid device %q: must be one of auto, cpu, gpu", device)
	}
}
