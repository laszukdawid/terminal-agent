package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		assert.Equal(t, DefaultBedrockModel, config.Providers["bedrock"])
		assert.Equal(t, "gpt-4o-mini", config.Providers["codex"])
		assert.Equal(t, "llama3.2", config.Providers["llama"])
		assert.Equal(t, "mimo-v2.5-pro", config.Providers["mimo"])
		assert.Equal(t, "mistral-small-latest", config.Providers["mistral"])
		assert.Equal(t, "gpt-4o-mini", config.Providers["openai"])
		assert.Equal(t, "gemini-3.1-flash-lite", config.Providers["google"])
		assert.Equal(t, "llama3.2", config.Providers["ollama"])
		assert.Empty(t, config.GetLlamaModels())
		assert.Empty(t, config.GetBedrockProfile())
		assert.Empty(t, config.GetBedrockRegion())
		input, output, lastChecked, ok := config.GetBedrockModelPrice("us-east-1", DefaultBedrockModel)
		require.True(t, ok)
		assert.Equal(t, 0.00007, input)
		assert.Equal(t, 0.0004, output)
		_, parseErr := time.Parse(time.RFC3339, lastChecked)
		assert.NoError(t, parseErr)

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

	t.Run("LoadBedrockConfig", func(t *testing.T) {
		_, cleanup := setupTempConfig(t)
		defer cleanup()

		expectedConfig := &config{
			DefaultProvider: "bedrock",
			Providers: map[string]string{
				"bedrock": DefaultBedrockModel,
			},
			Bedrock: BedrockConfig{Profile: " dev ", Region: " us-west-2 "},
		}
		require.NoError(t, SaveConfig(expectedConfig))

		loadedConfig, err := LoadConfig()
		require.NoError(t, err)

		assert.Equal(t, "dev", loadedConfig.GetBedrockProfile())
		assert.Equal(t, "us-west-2", loadedConfig.GetBedrockRegion())
	})

	t.Run("SetBedrockModelPrice", func(t *testing.T) {
		_, cleanup := setupTempConfig(t)
		defer cleanup()

		cfg := NewDefaultConfig()
		require.NoError(t, SaveConfig(cfg))

		require.NoError(t, cfg.SetBedrockModelPrice(" us-west-2 ", " model-id ", 0.001, 0.002, " 2026-06-09T00:00:00Z "))

		loadedConfig, err := LoadConfig()
		require.NoError(t, err)
		input, output, lastChecked, ok := loadedConfig.GetBedrockModelPrice("us-west-2", "model-id")
		require.True(t, ok)
		assert.Equal(t, 0.001, input)
		assert.Equal(t, 0.002, output)
		assert.Equal(t, "2026-06-09T00:00:00Z", lastChecked)
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

func TestGetModelIdForProvider(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Providers["google"] = "gemini-custom"
	cfg.Providers["anthropic"] = "claude-custom"
	cfg.DefaultProvider = "google"

	assert.Equal(t, "gemini-custom", cfg.GetDefaultModelId())
	assert.Equal(t, "gemini-custom", cfg.GetModelIdForProvider("google"))
	assert.Equal(t, "claude-custom", cfg.GetModelIdForProvider("anthropic"))
}

func TestDeviceDefaultsToAuto(t *testing.T) {
	cfg := NewDefaultConfig()
	assert.Equal(t, "auto", cfg.GetDevice())

	cfg.Device = ""
	assert.Equal(t, "auto", cfg.GetDevice())

	cfg.Device = "invalid"
	assert.Equal(t, "auto", cfg.GetDevice())
}

func TestGUIConfigDefaults(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cfg := NewDefaultConfig()

	assert.Equal(t, filepath.Join(homeDir, ".config", "terminal-agent", ".gui.env"), cfg.GetGUIEnvFile())
	assert.False(t, cfg.GetGUILoadShellEnvironment())
	assert.Equal(t, 2*time.Second, cfg.GetGUIShellEnvironmentTimeout())
	assert.True(t, cfg.GetGUIVoiceEnabled())
	assert.Equal(t, DefaultGUIVoiceTriggerKey, cfg.GetGUIVoiceTriggerKey())
	assert.True(t, cfg.GetGUIVoiceAutoSubmit())
	assert.Equal(t, DefaultGUIVoiceMaxRecordingDuration, cfg.GetGUIVoiceMaxRecordingDuration())
	assert.Equal(t, DefaultGUIVoiceSTTBackend, cfg.GetGUIVoiceSTTBackend())
	assert.Equal(t, DefaultGUIVoiceSTTModel, cfg.GetGUIVoiceSTTModel())
	assert.Equal(t, "", cfg.GetGUIVoiceSTTLanguage())
}

func TestLoadConfigGUIOverrides(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	configDir := filepath.Join(homeDir, ".config", "terminal-agent")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	loadShell := false
	content, err := json.Marshal(map[string]any{
		"default_provider": "openai",
		"providers": map[string]string{
			"openai": "gpt-4o-mini",
		},
		"gui": GUIConfig{
			EnvFile:                 "~/custom-gui.env",
			LoadShellEnvironment:    &loadShell,
			ShellEnvironmentTimeout: "500ms",
		},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.json"), content, 0o600))

	cfg, err := LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(homeDir, "custom-gui.env"), cfg.GetGUIEnvFile())
	assert.False(t, cfg.GetGUILoadShellEnvironment())
	assert.Equal(t, 500*time.Millisecond, cfg.GetGUIShellEnvironmentTimeout())
}

func TestGUIVoiceConfigOverrides(t *testing.T) {
	enabled := false
	autoSubmit := false
	cfg := NewDefaultConfig()
	cfg.GUI.Voice = GUIVoiceConfig{
		Enabled:              &enabled,
		TriggerKey:           "F2",
		AutoSubmit:           &autoSubmit,
		MaxRecordingDuration: "15s",
		STT: GUIVoiceSTTConfig{
			Backend:  "openai",
			Model:    "whisper-1",
			Language: "en",
		},
	}

	assert.False(t, cfg.GetGUIVoiceEnabled())
	assert.Equal(t, "F2", cfg.GetGUIVoiceTriggerKey())
	assert.False(t, cfg.GetGUIVoiceAutoSubmit())
	assert.Equal(t, 15*time.Second, cfg.GetGUIVoiceMaxRecordingDuration())
	assert.Equal(t, "openai", cfg.GetGUIVoiceSTTBackend())
	assert.Equal(t, "whisper-1", cfg.GetGUIVoiceSTTModel())
	assert.Equal(t, "en", cfg.GetGUIVoiceSTTLanguage())
}

func TestGUIVoiceMaxRecordingDurationFallback(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "invalid duration", value: "not-a-duration"},
		{name: "zero duration", value: "0s"},
		{name: "negative duration", value: "-1s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewDefaultConfig()
			cfg.GUI.Voice.MaxRecordingDuration = tt.value
			assert.Equal(t, DefaultGUIVoiceMaxRecordingDuration, cfg.GetGUIVoiceMaxRecordingDuration())
		})
	}
}

func TestSetDevice(t *testing.T) {
	_, cleanup := setupTempConfig(t)
	defer cleanup()

	cfg := NewDefaultConfig()

	require.NoError(t, cfg.SetDevice("cpu"))
	assert.Equal(t, "cpu", cfg.GetDevice())

	require.NoError(t, cfg.SetDevice("gpu"))
	assert.Equal(t, "gpu", cfg.GetDevice())

	require.NoError(t, cfg.SetDevice("auto"))
	assert.Equal(t, "auto", cfg.GetDevice())

	err := cfg.SetDevice("tpu")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be one of auto, cpu, gpu")
}

func TestGetProjectContext(t *testing.T) {
	t.Run("defaults to true when nil", func(t *testing.T) {
		cfg := NewDefaultConfig()
		assert.True(t, cfg.GetProjectContext())
	})

	t.Run("returns true when explicitly set", func(t *testing.T) {
		cfg := NewDefaultConfig()
		val := true
		cfg.ProjectContext = &val
		assert.True(t, cfg.GetProjectContext())
	})

	t.Run("returns false when explicitly disabled", func(t *testing.T) {
		cfg := NewDefaultConfig()
		val := false
		cfg.ProjectContext = &val
		assert.False(t, cfg.GetProjectContext())
	})
}

func TestGetTaskTimeout(t *testing.T) {
	tests := []struct {
		name        string
		taskTimeout string
		want        time.Duration
	}{
		{name: "defaults to zero when unset", want: 0},
		{name: "parses valid duration", taskTimeout: "15m", want: 15 * time.Minute},
		{name: "returns zero on invalid duration", taskTimeout: "not-a-duration", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewDefaultConfig()
			cfg.TaskTimeout = tt.taskTimeout
			assert.Equal(t, tt.want, cfg.GetTaskTimeout())
		})
	}
}

func TestGetTaskLiveOutputLimit(t *testing.T) {
	tests := []struct {
		name  string
		value *int
		want  int
	}{
		{name: "defaults to configured default when unset", want: DefaultTaskLiveOutputLimit},
		{name: "returns configured positive value", value: intPtr(12), want: 12},
		{name: "allows zero for unlimited", value: intPtr(0), want: 0},
		{name: "defaults negative values", value: intPtr(-1), want: DefaultTaskLiveOutputLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewDefaultConfig()
			cfg.TaskLiveOutputLimit = tt.value
			assert.Equal(t, tt.want, cfg.GetTaskLiveOutputLimit())
		})
	}
}

func intPtr(v int) *int {
	return &v
}
