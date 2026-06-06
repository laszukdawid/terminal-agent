package stt

import (
	"testing"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNewTranscriberRejectsUnsupportedBackend(t *testing.T) {
	_, err := NewTranscriber(factoryConfig{backend: "local"})
	require.Error(t, err)
}

type factoryConfig struct {
	backend string
}

func (c factoryConfig) GetDefaultProvider() string                     { return "" }
func (c factoryConfig) GetDefaultModelId() string                      { return "" }
func (c factoryConfig) GetModelIdForProvider(string) string            { return "" }
func (c factoryConfig) GetLlamaModels() map[string]string              { return nil }
func (c factoryConfig) GetDevice() string                              { return "auto" }
func (c factoryConfig) GetGUIEnvFile() string                          { return "" }
func (c factoryConfig) GetGUILoadShellEnvironment() bool               { return false }
func (c factoryConfig) GetGUIShellEnvironmentTimeout() time.Duration   { return time.Second }
func (c factoryConfig) GetGUIVoiceEnabled() bool                       { return true }
func (c factoryConfig) GetGUIVoiceTriggerKey() string                  { return config.DefaultGUIVoiceTriggerKey }
func (c factoryConfig) GetGUIVoiceAutoSubmit() bool                    { return true }
func (c factoryConfig) GetGUIVoiceMaxRecordingDuration() time.Duration { return time.Minute }
func (c factoryConfig) GetGUIVoiceSTTBackend() string                  { return c.backend }
func (c factoryConfig) GetGUIVoiceSTTModel() string                    { return config.DefaultGUIVoiceSTTModel }
func (c factoryConfig) GetGUIVoiceSTTLanguage() string                 { return "" }
func (c factoryConfig) SetDefaultProvider(string) error                { return nil }
func (c factoryConfig) SetDefaultModelId(string) error                 { return nil }
func (c factoryConfig) SetDevice(string) error                         { return nil }
func (c factoryConfig) GetMcpFilePath() string                         { return "" }
func (c factoryConfig) SetMcpFilePath(string) error                    { return nil }
func (c factoryConfig) GetConfiguredWorkingDir() string                { return "" }
func (c factoryConfig) GetWorkingDir() string                          { return "" }
func (c factoryConfig) SetWorkingDir(string) error                     { return nil }
func (c factoryConfig) GetMaxTokens() int                              { return 0 }
func (c factoryConfig) GetTaskTimeout() time.Duration                  { return 0 }
func (c factoryConfig) GetTaskLiveOutputLimit() int                    { return 0 }
func (c factoryConfig) GetMemory() bool                                { return false }
func (c factoryConfig) SetMemory(bool) error                           { return nil }
func (c factoryConfig) GetPermissions() config.Permissions             { return config.Permissions{} }
func (c factoryConfig) GetProjectContext() bool                        { return false }
