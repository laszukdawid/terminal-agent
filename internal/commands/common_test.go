package commands

import (
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDevice(t *testing.T) {
	newFlags := func() *pflag.FlagSet {
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		flags.String("device", "", "")
		return flags
	}

	t.Run("defaults to config", func(t *testing.T) {
		flags := newFlags()
		cfg := config.NewDefaultConfig()
		cfg.Device = "cpu"

		device, err := resolveDevice(flags, cfg)
		require.NoError(t, err)
		assert.Equal(t, "cpu", device)
	})

	t.Run("defaults to auto without config", func(t *testing.T) {
		flags := newFlags()

		device, err := resolveDevice(flags, nil)
		require.NoError(t, err)
		assert.Equal(t, "auto", device)
	})

	t.Run("cli overrides config", func(t *testing.T) {
		flags := newFlags()
		require.NoError(t, flags.Set("device", "gpu"))
		cfg := config.NewDefaultConfig()
		cfg.Device = "cpu"

		device, err := resolveDevice(flags, cfg)
		require.NoError(t, err)
		assert.Equal(t, "gpu", device)
	})

	t.Run("rejects invalid cli value", func(t *testing.T) {
		flags := newFlags()
		require.NoError(t, flags.Set("device", "tpu"))

		_, err := resolveDevice(flags, config.NewDefaultConfig())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be one of auto, cpu, gpu")
	})
}
