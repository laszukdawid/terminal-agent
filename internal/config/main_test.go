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

// setupTempConfig creates a temporary directory for testing and returns cleanup function
func setupTempConfig(t *testing.T) (string, func()) {
	// Create a temporary directory to act as the HOME directory
	tempDir := t.TempDir()

	// Store original HOME value
	originalHome := os.Getenv("HOME")

	// Set HOME to temp directory
	os.Setenv("HOME", tempDir)

	// Define the config path
	configPath := filepath.Join(tempDir, ".config", "terminal-agent", "config.json")

	// Ensure the config file does not exist initially
	if _, err := os.Stat(configPath); err == nil {
		os.Remove(configPath)
	}

	// Return cleanup function
	cleanup := func() {
		// Restore original HOME
		os.Setenv("HOME", originalHome)
		// Temp directory is automatically cleaned up by t.TempDir()
	}

	return configPath, cleanup
}

func TestLoadConfig(t *testing.T) {
	// Test default config creation
	t.Run("DefaultConfigCreation", func(t *testing.T) {
		configPath, cleanup := setupTempConfig(t)
		defer cleanup()

		// Call LoadConfig
		config, err := LoadConfig()
		if err != nil {
			t.Fatalf("Expected no error, but got %v", err)
		}

		// Verify the default config values
		assert.Equal(t, "openai", config.GetDefaultProvider())
		assert.Equal(t, "gpt-4o-mini", config.GetDefaultModelId())

		// Verify that default providers included
		assert.Equal(t, "claude-3-5-haiku-latest", config.Providers["anthropic"])
		assert.Equal(t, "anthropic.claude-3-haiku-20240307-v1:0", config.Providers["bedrock"])
		assert.Equal(t, "llama-3-8b-instruct", config.Providers["perplexity"])
		assert.Equal(t, "gpt-4o-mini", config.Providers["openai"])
		assert.Equal(t, "gemini-2.0-flash-lite", config.Providers["google"])
		assert.Equal(t, "llama3.2", config.Providers["ollama"])

		// Verify config file was created
		_, err = os.Stat(configPath)
		assert.NoError(t, err, "Config file should exist after LoadConfig")
	})

	// Test loading existing config
	t.Run("LoadExistingConfig", func(t *testing.T) {
		_, cleanup := setupTempConfig(t)
		defer cleanup()

		// Create a config file with known values
		expectedConfig := &config{
			DefaultProvider: "ollama",
			Providers: map[string]string{
				"openai": "gpt-3.5-turbo",
				"ollama": "gemma3:27b",
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

	// Test Ollama provider configuration
	t.Run("OllamaProviderConfig", func(t *testing.T) {
		_, cleanup := setupTempConfig(t)
		defer cleanup()

		// Load default config
		config, err := LoadConfig()
		if err != nil {
			t.Fatalf("Expected no error, but got %v", err)
		}

		// Test setting Ollama as default provider
		err = config.SetDefaultProvider("ollama")
		if err != nil {
			t.Fatalf("Failed to set ollama provider: %v", err)
		}

		// Verify Ollama is now the default provider
		assert.Equal(t, "ollama", config.GetDefaultProvider())
		assert.Equal(t, "llama3.2", config.GetDefaultModelId())

		// Test setting a different Ollama model
		err = config.SetDefaultModelId("llama3.1")
		if err != nil {
			t.Fatalf("Failed to set ollama model: %v", err)
		}

		// Verify the model was updated
		assert.Equal(t, "llama3.1", config.GetDefaultModelId())
		assert.Equal(t, "llama3.1", config.Providers["ollama"])
	})
}
