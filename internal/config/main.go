package config

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func getConfigPath() string {
	return filepath.Join(os.Getenv("HOME"), ".config", "terminal-agent", "config.json")
}

type Config interface {
	GetDefaultProvider() string
	GetDefaultModelId() string
	SetDefaultProvider(string) error
	SetDefaultModelId(string) error
	GetMcpFilePath() string
	SetMcpFilePath(string) error
	// GetMaxTokens() int
	// SetMaxTokens(int) error
}

type config struct {
	LogLevel        string
	DefaultProvider string            `json:"default_provider"`
	Providers       map[string]string `json:"providers"`
	McpFilePath     string            `json:"mcp_file_path"`
	MaxTokens       int               `json:"max_tokens"`
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

func NewDefaultConfig() *config {
	return &config{
		DefaultProvider: "bedrock",
		Providers: map[string]string{
			"anthropic":  "anthropic.claude-3-haiku-20240307-v1:0",
			"bedrock":    "anthropic.claude-3-haiku-20240307-v1:0",
			"perplexity": "llama-3-8b-instruct",
			"openai":     "gpt-4o-mini",
		},
		LogLevel:    "info",
		McpFilePath: "",
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
	log.Println("Setting default provider to:", provider)
	config.DefaultProvider = provider
	return SaveConfig(config)
}

func (config *config) GetDefaultModelId() string {
	return config.Providers[config.DefaultProvider]
}

func (config *config) SetDefaultModelId(modelId string) error {
	log.Println("Setting default model ID to:", modelId)
	config.Providers[config.DefaultProvider] = modelId
	return SaveConfig(config)
}

func (config *config) GetMcpFilePath() string {
	return config.McpFilePath
}

func (config *config) SetMcpFilePath(path string) error {
	log.Println("Setting MCP file path to:", path)
	config.McpFilePath = path
	return SaveConfig(config)
}

var SetProviderCmd = &cobra.Command{
	Use:   "set-provider [provider]",
	Short: "Set the default provider",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		config, err := LoadConfig()
		if err != nil {
			fmt.Println("Error loading config:", err)
			return
		}
		provider := args[0]
		if err := config.SetDefaultProvider(provider); err != nil {
			fmt.Println("Error setting provider:", err)
		} else {
			fmt.Println("Default provider set to:", provider)
		}
	},
}

var GetConfigCmd = &cobra.Command{
	Use:   "get-config",
	Short: "Get the current configuration",
	Run: func(cmd *cobra.Command, args []string) {
		config, err := LoadConfig()
		if err != nil {
			fmt.Println("Error loading config:", err)
		} else {
			fmt.Println("Config:", config)
		}
	},
}
