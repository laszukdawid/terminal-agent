package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRoutineDefaultsAppliesBuiltins(t *testing.T) {
	cfg := NewDefaultConfig()

	defaults := cfg.GetRoutineDefaults()
	assert.Equal(t, DefaultRoutineTimeout, defaults.Timeout, "timeout falls back to the built-in 15m")
	require.NotNil(t, defaults.TokenBudget)
	assert.Equal(t, DefaultRoutineTokenBudget, *defaults.TokenBudget, "token budget falls back to the built-in 1M")
	assert.Nil(t, defaults.MaxTurns, "step limits are left unset so the agent default applies")
}

func TestGetRoutineDefaultsPreservesConfigured(t *testing.T) {
	budget := 250
	cfg := NewDefaultConfig()
	cfg.Routines.Defaults = RoutineDefaults{Timeout: "5m", TokenBudget: &budget, Provider: "anthropic"}

	defaults := cfg.GetRoutineDefaults()
	assert.Equal(t, "5m", defaults.Timeout)
	require.NotNil(t, defaults.TokenBudget)
	assert.Equal(t, 250, *defaults.TokenBudget)
	assert.Equal(t, "anthropic", defaults.Provider)
}

func TestGetRoutinesEnabledDefaultsTrue(t *testing.T) {
	cfg := NewDefaultConfig()
	assert.True(t, cfg.GetRoutinesEnabled())

	disabled := false
	cfg.Routines.Enabled = &disabled
	assert.False(t, cfg.GetRoutinesEnabled())
}

func TestSetRoutineDefaultsPersists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := NewDefaultConfig()
	budget := 777
	require.NoError(t, cfg.SetRoutineDefaults(RoutineDefaults{Model: "gpt-4o-mini", TokenBudget: &budget}))

	reloaded, err := LoadConfig()
	require.NoError(t, err)
	defaults := reloaded.GetRoutineDefaults()
	assert.Equal(t, "gpt-4o-mini", defaults.Model)
	require.NotNil(t, defaults.TokenBudget)
	assert.Equal(t, 777, *defaults.TokenBudget)
}
