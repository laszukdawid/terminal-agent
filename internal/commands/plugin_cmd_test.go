package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureBashReaderSourceBlockIsIdempotent(t *testing.T) {
	tempDir := t.TempDir()
	bashRCPath := filepath.Join(tempDir, ".bashrc")

	require.NoError(t, os.WriteFile(bashRCPath, []byte("export PATH=\"$PATH:$HOME/.local/bin\""), 0644))

	updated, err := ensureBashReaderSourceBlock(bashRCPath)
	require.NoError(t, err)
	assert.True(t, updated)

	updated, err = ensureBashReaderSourceBlock(bashRCPath)
	require.NoError(t, err)
	assert.False(t, updated)

	content, err := os.ReadFile(bashRCPath)
	require.NoError(t, err)

	text := string(content)
	assert.Equal(t, 1, strings.Count(text, bashReaderBlockStart))
	assert.Equal(t, 1, strings.Count(text, bashReaderBlockEnd))
	assert.Equal(t, 1, strings.Count(text, bashReaderSourceLine))
}

func TestInstallBashReaderPlugin(t *testing.T) {
	tempDir := t.TempDir()

	result, err := installBashReaderPlugin(tempDir)
	require.NoError(t, err)
	assert.True(t, result.BashRCUpdated)

	if _, err := os.Stat(result.ScriptPath); err != nil {
		t.Fatalf("expected plugin script to exist: %v", err)
	}

	bashRCContent, err := os.ReadFile(result.BashRCPath)
	require.NoError(t, err)
	assert.Contains(t, string(bashRCContent), bashReaderSourceLine)

	sessionDir := filepath.Join(getTerminalContextDir(tempDir), "sessions")
	if _, err := os.Stat(sessionDir); err != nil {
		t.Fatalf("expected session directory to exist: %v", err)
	}
}

func TestEnsureBashReaderSourceBlockAddedAfterExistingTerminalAgentBlock(t *testing.T) {
	tempDir := t.TempDir()
	bashRCPath := filepath.Join(tempDir, ".bashrc")

	existing := strings.Join([]string{
		"export PATH=\"$PATH:$HOME/.local/bin\"",
		"# BEGIN terminal-agent alias",
		"alias aa=\"agent ask\"",
		"# END terminal-agent alias",
		"export EDITOR=vim",
	}, "\n") + "\n"

	require.NoError(t, os.WriteFile(bashRCPath, []byte(existing), 0644))

	updated, err := ensureBashReaderSourceBlock(bashRCPath)
	require.NoError(t, err)
	assert.True(t, updated)

	content, err := os.ReadFile(bashRCPath)
	require.NoError(t, err)

	text := string(content)
	firstEnd := strings.Index(text, "# END terminal-agent alias")
	newStart := strings.Index(text, bashReaderBlockStart+"\n"+bashReaderSourceLine)
	editorLine := strings.Index(text, "export EDITOR=vim")

	require.NotEqual(t, -1, firstEnd)
	require.NotEqual(t, -1, newStart)
	require.NotEqual(t, -1, editorLine)
	assert.Greater(t, newStart, firstEnd)
	assert.Greater(t, editorLine, newStart)
}

func TestUninstallBashReaderPluginRemovesOnlyPluginBlock(t *testing.T) {
	tempDir := t.TempDir()
	bashRCPath := filepath.Join(tempDir, ".bashrc")

	content := strings.Join([]string{
		"export PATH=\"$PATH:$HOME/.local/bin\"",
		"# BEGIN terminal-agent alias",
		"alias aa=\"agent ask\"",
		"# END terminal-agent alias",
		bashReaderBlockStart,
		bashReaderSourceLine,
		bashReaderBlockEnd,
		"export EDITOR=vim",
	}, "\n") + "\n"
	require.NoError(t, os.WriteFile(bashRCPath, []byte(content), 0644))

	pluginDir := filepath.Dir(getBashReaderScriptPath(tempDir))
	require.NoError(t, os.MkdirAll(pluginDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "init.bash"), []byte("hook"), 0644))

	contextDir := getTerminalContextDir(tempDir)
	require.NoError(t, os.MkdirAll(contextDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "index.log"), []byte("log"), 0644))

	result, err := uninstallBashReaderPlugin(tempDir, false)
	require.NoError(t, err)
	assert.True(t, result.BashRCUpdated)
	assert.True(t, result.PluginDirRemoved)
	assert.False(t, result.ContextDirRemoved)

	updatedContent, err := os.ReadFile(bashRCPath)
	require.NoError(t, err)
	text := string(updatedContent)
	assert.Contains(t, text, "# BEGIN terminal-agent alias")
	assert.Contains(t, text, "# END terminal-agent alias")
	assert.NotContains(t, text, bashReaderBlockStart)
	assert.NotContains(t, text, bashReaderSourceLine)

	_, err = os.Stat(pluginDir)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(contextDir)
	assert.NoError(t, err)
}

func TestUninstallBashReaderPluginWithPurgeData(t *testing.T) {
	tempDir := t.TempDir()

	_, err := installBashReaderPlugin(tempDir)
	require.NoError(t, err)

	contextDir := getTerminalContextDir(tempDir)
	require.NoError(t, os.MkdirAll(contextDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(contextDir, "index.log"), []byte("log"), 0644))

	result, err := uninstallBashReaderPlugin(tempDir, true)
	require.NoError(t, err)
	assert.True(t, result.ContextDirRemoved)

	_, err = os.Stat(contextDir)
	assert.True(t, os.IsNotExist(err))
}

func TestBashReaderScriptDoesNotOverrideStdoutStderr(t *testing.T) {
	assert.NotContains(t, bashReaderInitScript, "exec > >(")
	assert.NotContains(t, bashReaderInitScript, "exec 1>&")
	assert.NotContains(t, bashReaderInitScript, "tee -a")
}
