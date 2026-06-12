package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadGlobalPermissionsIgnoresUnrelatedConfigShape(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	configDir := filepath.Join(homeDir, ".config", "terminal-agent")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{
		"permissions": {"allow": ["unix(\"git remote -v\")"]},
		"bedrock": {"prices": {"us-east-1": {"model-id": 0.001}}}
	}`), 0o600))

	permissions, err := loadGlobalPermissions()
	require.NoError(t, err)
	assert.Equal(t, []string{`unix("git remote -v")`}, permissions.Allow)
}
