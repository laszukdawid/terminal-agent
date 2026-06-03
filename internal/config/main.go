package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	log "github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/spf13/cobra"
)

func getConfigPath() string {
	return filepath.Join(os.Getenv("HOME"), ".config", "terminal-agent", "config.json")
}

type Config interface {
	GetDefaultProvider() string
	GetDefaultModelId() string
	GetModelIdForProvider(string) string
	GetLlamaModels() map[string]string
	GetDevice() string
	GetGUIEnvFile() string
	GetGUILoadShellEnvironment() bool
	GetGUIShellEnvironmentTimeout() time.Duration
	SetDefaultProvider(string) error
	SetDefaultModelId(string) error
	SetDevice(string) error
	GetMcpFilePath() string
	SetMcpFilePath(string) error
	GetConfiguredWorkingDir() string
	GetWorkingDir() string
	SetWorkingDir(string) error
	GetMaxTokens() int
	GetTaskTimeout() time.Duration
	GetMemory() bool
	SetMemory(bool) error
	GetPermissions() Permissions
	GetProjectContext() bool
}

type config struct {
	LogLevel        string
	DefaultProvider string            `json:"default_provider"`
	Providers       map[string]string `json:"providers"`
	LlamaModels     map[string]string `json:"llama_models,omitempty"`
	GUI             GUIConfig         `json:"gui,omitempty"`
	Device          string            `json:"device,omitempty"`
	McpFilePath     string            `json:"mcp_file_path"`
	WorkingDir      string            `json:"working_dir"`
	MaxTokens       int               `json:"max_tokens"`
	TaskTimeout     string            `json:"task_timeout,omitempty"`
	Memory          bool              `json:"memory"`
	ProjectContext  *bool             `json:"project_context,omitempty"`
	Permissions     Permissions       `json:"permissions,omitempty"`
}

type GUIConfig struct {
	EnvFile                 string `json:"env_file,omitempty"`
	LoadShellEnvironment    *bool  `json:"load_shell_environment,omitempty"`
	ShellEnvironmentTimeout string `json:"shell_environment_timeout,omitempty"`
}

func ensurePathExists(path string) error {
	dir := filepath.Dir(path)
	// Check if the directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// Create the directory
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return err
		}
	}

	// Check if the file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Create the file
		if _, err := os.Create(path); err != nil {
			return err
		}
	}
	return nil
}

func getDefaultWorkingDir() string {
	return filepath.Dir(getConfigPath())
}

func getDefaultGUIEnvFile() string {
	return filepath.Join(filepath.Dir(getConfigPath()), ".gui.env")
}

func NewDefaultConfig() *config {
	return &config{
		DefaultProvider: "openai",
		Providers: map[string]string{
			"anthropic": "claude-3-5-haiku-latest",
			"bedrock":   "anthropic.claude-3-haiku-20240307-v1:0",
			"codex":     "gpt-4o-mini",
			"google":    "gemini-3.1-flash-lite",
			"llama":     "llama3.2",
			"mimo":      "mimo-v2.5-pro",
			"mistral":   "mistral-small-latest",
			"openai":    "gpt-4o-mini",
			"ollama":    "llama3.2",
		},
		LlamaModels: map[string]string{},
		LogLevel:    "info",
		Device:      "auto",
		McpFilePath: "",
		WorkingDir:  "",
		MaxTokens:   600,
	}
}

func LoadConfig() (*config, error) {
	configPath := getConfigPath()
	if err := ensurePathExists(configPath); err != nil {
		return nil, err
	}

	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}

	content, err := io.ReadAll(file)
	file.Close()
	if err != nil {
		return nil, err
	}

	if len(content) == 0 {
		c := NewDefaultConfig()
		SaveConfig(c)
		return c, nil
	} else {
		// Decode the JSON content
		config := &config{}
		if err := json.Unmarshal(content, config); err != nil {
			return nil, err
		}
		return config, nil
	}
}

func SaveConfig(config *config) error {
	configPath := getConfigPath()
	if err := ensurePathExists(configPath); err != nil {
		return err
	}
	file, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(config)
}

func (config *config) GetDefaultProvider() string {
	return config.DefaultProvider
}

func (config *config) SetDefaultProvider(provider string) error {
	log.Debugw("Setting default provider", "provider", provider)
	config.DefaultProvider = provider
	return SaveConfig(config)
}

func (config *config) GetDefaultModelId() string {
	return config.GetModelIdForProvider(config.DefaultProvider)
}

func (config *config) GetModelIdForProvider(provider string) string {
	return config.Providers[provider]
}

func (config *config) GetLlamaModels() map[string]string {
	if config.LlamaModels == nil {
		return map[string]string{}
	}
	return config.LlamaModels
}

func normalizeDevice(device string) string {
	switch device {
	case "", "auto":
		return "auto"
	case "cpu", "gpu":
		return device
	default:
		return ""
	}
}

func (config *config) GetDevice() string {
	device := normalizeDevice(config.Device)
	if device == "" {
		return "auto"
	}
	return device
}

func (config *config) GetGUIEnvFile() string {
	if config.GUI.EnvFile == "" {
		return getDefaultGUIEnvFile()
	}
	return expandHome(config.GUI.EnvFile)
}

func (config *config) GetGUILoadShellEnvironment() bool {
	if config.GUI.LoadShellEnvironment == nil {
		return false
	}
	return *config.GUI.LoadShellEnvironment
}

func (config *config) GetGUIShellEnvironmentTimeout() time.Duration {
	if config.GUI.ShellEnvironmentTimeout == "" {
		return 2 * time.Second
	}
	d, err := time.ParseDuration(config.GUI.ShellEnvironmentTimeout)
	if err != nil || d <= 0 {
		log.Warnw("Invalid gui.shell_environment_timeout in config, using default", "value", config.GUI.ShellEnvironmentTimeout, "error", err)
		return 2 * time.Second
	}
	return d
}

func expandHome(path string) string {
	if path == "~" {
		return os.Getenv("HOME")
	}
	if len(path) > 2 && path[:2] == "~/" {
		return filepath.Join(os.Getenv("HOME"), path[2:])
	}
	return path
}

func (config *config) SetDefaultModelId(modelId string) error {
	log.Debugw("Setting default model ID", "model", modelId)
	config.Providers[config.DefaultProvider] = modelId
	return SaveConfig(config)
}

func (config *config) SetDevice(device string) error {
	normalized := normalizeDevice(device)
	if normalized == "" {
		return fmt.Errorf("invalid device %q: must be one of auto, cpu, gpu", device)
	}
	log.Debugw("Setting device", "device", normalized)
	config.Device = normalized
	return SaveConfig(config)
}

func (config *config) GetMcpFilePath() string {
	return config.McpFilePath
}

func (config *config) SetMcpFilePath(path string) error {
	log.Debugw("Setting MCP file path", "path", path)
	config.McpFilePath = path
	return SaveConfig(config)
}

func (config *config) GetWorkingDir() string {
	if config.WorkingDir == "" {
		return getDefaultWorkingDir()
	}
	return config.WorkingDir
}

func (config *config) GetConfiguredWorkingDir() string {
	return config.WorkingDir
}

func (config *config) GetMaxTokens() int {
	if config.MaxTokens <= 0 {
		return NewDefaultConfig().MaxTokens
	}
	return config.MaxTokens
}

// GetTaskTimeout returns the configured default task timeout. An empty value or
// an unparseable duration string yields 0, which the agent treats as unlimited.
func (config *config) GetTaskTimeout() time.Duration {
	if config.TaskTimeout == "" {
		return 0
	}
	d, err := time.ParseDuration(config.TaskTimeout)
	if err != nil {
		log.Warnw("Invalid task_timeout in config, treating as unlimited", "value", config.TaskTimeout, "error", err)
		return 0
	}
	return d
}

func (config *config) SetWorkingDir(path string) error {
	log.Debugw("Setting working directory", "path", path)
	config.WorkingDir = path
	return SaveConfig(config)
}

func (config *config) GetMemory() bool {
	return config.Memory
}

func (config *config) SetMemory(enabled bool) error {
	log.Debugw("Setting memory", "enabled", enabled)
	config.Memory = enabled
	return SaveConfig(config)
}

func (config *config) GetPermissions() Permissions {
	return config.Permissions
}

func (config *config) GetProjectContext() bool {
	if config.ProjectContext == nil {
		return true
	}
	return *config.ProjectContext
}

var SetProviderCmd = &cobra.Command{
	Use:   "set-provider [provider]",
	Short: "Set the default provider",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := LoadConfig()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		provider := args[0]
		if err := config.SetDefaultProvider(provider); err != nil {
			return fmt.Errorf("set provider: %w", err)
		}
		cmd.Printf("Default provider set to: %s\n", provider)
		return nil
	},
}

var GetConfigCmd = &cobra.Command{
	Use:   "get-config",
	Short: "Get the current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := LoadConfig()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		cmd.Printf("Config: %+v\n", config)
		return nil
	},
}
