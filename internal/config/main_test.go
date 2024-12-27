package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetConfigPath(t *testing.T) {
	// Set the HOME environment variable to a known value
	homeDir := "/home/testuser"
	os.Setenv("HOME", homeDir)

	// Call getConfigPath
	configPath := getConfigPath()

	// Expected path
	expectedPath := filepath.Join(homeDir, ".config", "terminal-agent", "config.json")

	// Verify the returned path matches the expected path
	assert.Equal(t, expectedPath, configPath)
}

func TestLoadConfig(t *testing.T) {
	// Create a temporary directory to act as the HOME directory
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)

	// Define the config path
	configPath := filepath.Join(tempDir, ".config", "terminal-agent", "config.json")

	// Test default config creation
	t.Run("DefaultConfigCreation", func(t *testing.T) {
		// Ensure the config file does not exist
		if _, err := os.Stat(configPath); err == nil {
			os.Remove(configPath)
		}

		// Call LoadConfig
		config, err := LoadConfig()
		if err != nil {
			t.Fatalf("Expected no error, but got %v", err)
		}

		// Verify the default config values
		assert.Equal(t, "bedrock", config.GetDefaultProvider())
		assert.Equal(t, "anthropic.claude-3-haiku-20240307-v1:0", config.GetDefaultModelId())

		// Verify that default providers included
		assert.Equal(t, "anthropic.claude-3-haiku-20240307-v1:0", config.Providers["anthropic"])
		assert.Equal(t, "anthropic.claude-3-haiku-20240307-v1:0", config.Providers["bedrock"])
		assert.Equal(t, "llama-3-8b-instruct", config.Providers["perplexity"])
		assert.Equal(t, "gpt-4o-mini", config.Providers["openai"])

	})

	// Test loading existing config
	t.Run("LoadExistingConfig", func(t *testing.T) {
		// Create a config file with known values
		expectedConfig := &config{
			DefaultProvider: "openai",
			Providers: map[string]string{
				"openai": "gpt-3.5-turbo",
			},
		}
		if err := SaveConfig(expectedConfig); err != nil {
			t.Fatalf("Failed to save config: %v", err)
		}

		// Call LoadConfig
		loadedConfig, err := LoadConfig()
		if err != nil {
			t.Fatalf("Expected no error, but got %v", err)
		}

		// Verify the loaded config values
		assert.Equal(t, expectedConfig.DefaultProvider, loadedConfig.DefaultProvider)
		assert.Equal(t, expectedConfig.Providers, loadedConfig.Providers)
		assert.Equal(t, expectedConfig.GetDefaultModelId(), loadedConfig.GetDefaultModelId())
	})
}
