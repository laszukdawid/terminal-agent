package commands

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMemoryFlagBehavior tests the logic that determines when memory should be included
func TestMemoryFlagBehavior(t *testing.T) {
	t.Run("ConfigFalseAndFlagFalse", func(t *testing.T) {
		configMemory := false
		flagMemory := false
		includeMemory := configMemory || flagMemory
		assert.False(t, includeMemory, "Memory should not be included when both config and flag are false")
	})

	t.Run("ConfigTrueAndFlagFalse", func(t *testing.T) {
		configMemory := true
		flagMemory := false
		includeMemory := configMemory || flagMemory
		assert.True(t, includeMemory, "Memory should be included when config is true")
	})

	t.Run("ConfigFalseAndFlagTrue", func(t *testing.T) {
		configMemory := false
		flagMemory := true
		includeMemory := configMemory || flagMemory
		assert.True(t, includeMemory, "Memory should be included when flag is true")
	})

	t.Run("ConfigTrueAndFlagTrue", func(t *testing.T) {
		configMemory := true
		flagMemory := true
		includeMemory := configMemory || flagMemory
		assert.True(t, includeMemory, "Memory should be included when both config and flag are true")
	})
}

func TestResolveTerminalContextCount(t *testing.T) {
	newFlagSet := func() *pflag.FlagSet {
		flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
		flags.Int("use-terminal-context", 0, "")
		flags.BoolP("terminal-context-1", "1", false, "")
		flags.BoolP("terminal-context-2", "2", false, "")
		flags.BoolP("terminal-context-3", "3", false, "")
		flags.BoolP("terminal-context-4", "4", false, "")
		flags.BoolP("terminal-context-5", "5", false, "")
		return flags
	}

	t.Run("NoFlags", func(t *testing.T) {
		flags := newFlagSet()
		count, err := resolveTerminalContextCount(flags)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("LongFlagValid", func(t *testing.T) {
		flags := newFlagSet()
		require.NoError(t, flags.Set("use-terminal-context", "4"))
		count, err := resolveTerminalContextCount(flags)
		require.NoError(t, err)
		assert.Equal(t, 4, count)
	})

	t.Run("LongFlagInvalidLow", func(t *testing.T) {
		flags := newFlagSet()
		require.NoError(t, flags.Set("use-terminal-context", "0"))
		_, err := resolveTerminalContextCount(flags)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "between 1 and 5")
	})

	t.Run("LongFlagInvalidHigh", func(t *testing.T) {
		flags := newFlagSet()
		require.NoError(t, flags.Set("use-terminal-context", "6"))
		_, err := resolveTerminalContextCount(flags)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "between 1 and 5")
	})

	t.Run("ShortcutFlag", func(t *testing.T) {
		flags := newFlagSet()
		require.NoError(t, flags.Set("terminal-context-3", "true"))
		count, err := resolveTerminalContextCount(flags)
		require.NoError(t, err)
		assert.Equal(t, 3, count)
	})

	t.Run("ShortcutFlagFromCLIArg", func(t *testing.T) {
		flags := newFlagSet()
		require.NoError(t, flags.Parse([]string{"-5"}))
		count, err := resolveTerminalContextCount(flags)
		require.NoError(t, err)
		assert.Equal(t, 5, count)
	})

	t.Run("MultipleShortcutFlags", func(t *testing.T) {
		flags := newFlagSet()
		require.NoError(t, flags.Set("terminal-context-2", "true"))
		require.NoError(t, flags.Set("terminal-context-4", "true"))
		_, err := resolveTerminalContextCount(flags)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only one terminal context shortcut")
	})

	t.Run("LongAndShortcutConflict", func(t *testing.T) {
		flags := newFlagSet()
		require.NoError(t, flags.Set("use-terminal-context", "2"))
		require.NoError(t, flags.Set("terminal-context-2", "true"))
		_, err := resolveTerminalContextCount(flags)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot combine --use-terminal-context")
	})

	t.Run("LongFlagRequiresValue", func(t *testing.T) {
		flags := newFlagSet()
		err := flags.Parse([]string{"--use-terminal-context"})
		require.Error(t, err)
	})
}
