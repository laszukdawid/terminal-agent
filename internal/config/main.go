package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	GetGUIVoiceEnabled() bool
	GetGUIVoiceTriggerKey() string
	GetGUIVoiceAutoSubmit() bool
	GetGUIVoiceMaxRecordingDuration() time.Duration
	GetGUIVoiceSTTBackend() string
	GetGUIVoiceSTTModel() string
	GetGUIVoiceSTTLanguage() string
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
	GetTaskLiveOutputLimit() int
	GetMemory() bool
	SetMemory(bool) error
	GetPermissions() Permissions
	GetProjectContext() bool
}

const (
	DefaultTaskLiveOutputLimit          = 6
	DefaultGUIVoiceEnabled              = true
	DefaultGUIVoiceTriggerKey           = "F1"
	DefaultGUIVoiceAutoSubmit           = true
	DefaultGUIVoiceMaxRecordingDuration = 60 * time.Second
	DefaultGUIVoiceSTTBackend           = "openai"
	DefaultGUIVoiceSTTModel             = "gpt-4o-mini-transcribe"
	DefaultBedrockModel                 = "zai.glm-4.7-flash"
)

type config struct {
	LogLevel            string
	DefaultProvider     string            `json:"default_provider"`
	Providers           map[string]string `json:"providers"`
	Bedrock             BedrockConfig     `json:"bedrock,omitempty"`
	LlamaModels         map[string]string `json:"llama_models,omitempty"`
	GUI                 GUIConfig         `json:"gui,omitempty"`
	Device              string            `json:"device,omitempty"`
	McpFilePath         string            `json:"mcp_file_path"`
	WorkingDir          string            `json:"working_dir"`
	MaxTokens           int               `json:"max_tokens"`
	TaskTimeout         string            `json:"task_timeout,omitempty"`
	TaskLiveOutputLimit *int              `json:"task_live_output_limit,omitempty"`
	Memory              bool              `json:"memory"`
	ProjectContext      *bool             `json:"project_context,omitempty"`
	Permissions         Permissions       `json:"permissions,omitempty"`
}

type GUIConfig struct {
	EnvFile                 string         `json:"env_file,omitempty"`
	LoadShellEnvironment    *bool          `json:"load_shell_environment,omitempty"`
	ShellEnvironmentTimeout string         `json:"shell_environment_timeout,omitempty"`
	Voice                   GUIVoiceConfig `json:"voice,omitempty"`
}

type BedrockConfig struct {
	Profile string                                   `json:"profile,omitempty"`
	Region  string                                   `json:"region,omitempty"`
	Prices  map[string]map[string]BedrockPriceConfig `json:"prices,omitempty"`
}

type BedrockPriceConfig struct {
	InputPer1K  float64 `json:"input_per_1k"`
	OutputPer1K float64 `json:"output_per_1k"`
	LastChecked string  `json:"last_checked"`
}

type GUIVoiceConfig struct {
	Enabled              *bool             `json:"enabled,omitempty"`
	TriggerKey           string            `json:"trigger_key,omitempty"`
	AutoSubmit           *bool             `json:"auto_submit,omitempty"`
	MaxRecordingDuration string            `json:"max_recording_duration,omitempty"`
	STT                  GUIVoiceSTTConfig `json:"stt,omitempty"`
}

type GUIVoiceSTTConfig struct {
	Backend  string `json:"backend,omitempty"`
	Model    string `json:"model,omitempty"`
	Language string `json:"language,omitempty"`
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
			"bedrock":   DefaultBedrockModel,
			"codex":     "gpt-4o-mini",
			"google":    "gemini-3.1-flash-lite",
			"llama":     "llama3.2",
			"mimo":      "mimo-v2.5-pro",
			"mistral":   "mistral-small-latest",
			"openai":    "gpt-4o-mini",
			"ollama":    "llama3.2",
		},
		Bedrock: BedrockConfig{Prices: map[string]map[string]BedrockPriceConfig{
			"us-east-1": {
				DefaultBedrockModel: {
					InputPer1K:  0.00007,
					OutputPer1K: 0.0004,
					LastChecked: time.Now().UTC().Format(time.RFC3339),
				},
			},
		}},
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

func (config *config) GetBedrockProfile() string {
	return strings.TrimSpace(config.Bedrock.Profile)
}

func (config *config) GetBedrockRegion() string {
	return strings.TrimSpace(config.Bedrock.Region)
}

func (config *config) GetBedrockModelPrice(region string, modelID string) (float64, float64, string, bool) {
	if config.Bedrock.Prices == nil {
		return 0, 0, "", false
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}
	prices, ok := config.Bedrock.Prices[region]
	if !ok {
		return 0, 0, "", false
	}
	price, ok := prices[modelID]
	if !ok {
		return 0, 0, "", false
	}
	return price.InputPer1K, price.OutputPer1K, strings.TrimSpace(price.LastChecked), true
}

func (config *config) SetBedrockModelPrice(region string, modelID string, inputPer1K, outputPer1K float64, lastChecked string) error {
	region = strings.TrimSpace(region)
	if region == "" {
		region = "us-east-1"
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return fmt.Errorf("bedrock model ID cannot be empty")
	}
	if inputPer1K < 0 || outputPer1K < 0 {
		return fmt.Errorf("bedrock model prices cannot be negative")
	}
	if config.Bedrock.Prices == nil {
		config.Bedrock.Prices = map[string]map[string]BedrockPriceConfig{}
	}
	if config.Bedrock.Prices[region] == nil {
		config.Bedrock.Prices[region] = map[string]BedrockPriceConfig{}
	}
	config.Bedrock.Prices[region][modelID] = BedrockPriceConfig{
		InputPer1K:  inputPer1K,
		OutputPer1K: outputPer1K,
		LastChecked: strings.TrimSpace(lastChecked),
	}
	return SaveConfig(config)
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

func (config *config) GetGUIVoiceEnabled() bool {
	if config.GUI.Voice.Enabled == nil {
		return DefaultGUIVoiceEnabled
	}
	return *config.GUI.Voice.Enabled
}

func (config *config) GetGUIVoiceTriggerKey() string {
	key := strings.ToUpper(strings.TrimSpace(config.GUI.Voice.TriggerKey))
	if key == "" {
		return DefaultGUIVoiceTriggerKey
	}
	if !isKnownGUIVoiceTriggerKey(key) {
		log.Warnw("Invalid gui.voice.trigger_key in config, using default", "value", config.GUI.Voice.TriggerKey)
		return DefaultGUIVoiceTriggerKey
	}
	return key
}

func isKnownGUIVoiceTriggerKey(key string) bool {
	switch key {
	case "F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12":
		return true
	default:
		return false
	}
}

func (config *config) GetGUIVoiceAutoSubmit() bool {
	if config.GUI.Voice.AutoSubmit == nil {
		return DefaultGUIVoiceAutoSubmit
	}
	return *config.GUI.Voice.AutoSubmit
}

func (config *config) GetGUIVoiceMaxRecordingDuration() time.Duration {
	if config.GUI.Voice.MaxRecordingDuration == "" {
		return DefaultGUIVoiceMaxRecordingDuration
	}
	d, err := time.ParseDuration(config.GUI.Voice.MaxRecordingDuration)
	if err != nil || d <= 0 {
		log.Warnw("Invalid gui.voice.max_recording_duration in config, using default", "value", config.GUI.Voice.MaxRecordingDuration, "error", err)
		return DefaultGUIVoiceMaxRecordingDuration
	}
	return d
}

func (config *config) GetGUIVoiceSTTBackend() string {
	if config.GUI.Voice.STT.Backend == "" {
		return DefaultGUIVoiceSTTBackend
	}
	return config.GUI.Voice.STT.Backend
}

func (config *config) GetGUIVoiceSTTModel() string {
	if config.GUI.Voice.STT.Model == "" {
		return DefaultGUIVoiceSTTModel
	}
	return config.GUI.Voice.STT.Model
}

func (config *config) GetGUIVoiceSTTLanguage() string {
	return config.GUI.Voice.STT.Language
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

func (config *config) GetTaskLiveOutputLimit() int {
	if config.TaskLiveOutputLimit == nil {
		return DefaultTaskLiveOutputLimit
	}
	if *config.TaskLiveOutputLimit < 0 {
		log.Warnw("Invalid task_live_output_limit in config, using default", "value", *config.TaskLiveOutputLimit)
		return DefaultTaskLiveOutputLimit
	}
	return *config.TaskLiveOutputLimit
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
