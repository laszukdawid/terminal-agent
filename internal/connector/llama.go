package connector

import (
	"context"
	"fmt"
	"os"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
)

const (
	LlamaProvider     = "llama"
	DefaultLlamaModel = "llama3.2"
)

type LlamaConnector struct {
	logger    zap.Logger
	modelID   string
	modelPath string
	config    config.Config
}

func NewLlamaConnector(modelID *string, cfg config.Config) *LlamaConnector {
	logger := *utils.GetLogger()
	logger.Debug("NewLlamaConnector")

	if modelID == nil || *modelID == "" {
		model := DefaultLlamaModel
		modelID = &model
	}

	connector := &LlamaConnector{
		logger:  logger,
		modelID: *modelID,
		config:  cfg,
	}

	modelPath, err := resolveLlamaModelPath(*modelID, cfg)
	if err == nil {
		connector.modelPath = modelPath
	} else {
		logger.Debug("llama model path not resolved during initialization", zap.String("model", *modelID), zap.Error(err))
	}

	return connector
}

func (lc *LlamaConnector) Query(_ context.Context, _ *QueryParams) (string, error) {
	if _, err := lc.requireModelPath(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("llama provider is not implemented yet")
}

func (lc *LlamaConnector) QueryWithTool(_ context.Context, _ *QueryParams, _ map[string]tools.Tool) (LlmResponseWithTools, error) {
	if _, err := lc.requireModelPath(); err != nil {
		return LlmResponseWithTools{}, err
	}

	return LlmResponseWithTools{}, fmt.Errorf("llama provider does not support tool calling yet")
}

func (lc *LlamaConnector) requireModelPath() (string, error) {
	if lc.modelPath != "" {
		return lc.modelPath, nil
	}

	modelPath, err := resolveLlamaModelPath(lc.modelID, lc.config)
	if err != nil {
		return "", err
	}

	lc.modelPath = modelPath
	return modelPath, nil
}

func resolveLlamaModelPath(modelID string, cfg config.Config) (string, error) {
	if modelID == "" {
		return "", fmt.Errorf("llama model cannot be empty")
	}

	if fileExists(modelID) {
		return modelID, nil
	}

	if cfg == nil {
		return "", fmt.Errorf("llama model %q is not a readable file path and no llama_models alias config is available", modelID)
	}

	llamaModels := cfg.GetLlamaModels()
	modelPath, ok := llamaModels[modelID]
	if !ok || modelPath == "" {
		return "", fmt.Errorf("llama model %q is not configured in llama_models and is not a readable file path", modelID)
	}

	if !fileExists(modelPath) {
		return "", fmt.Errorf("llama model %q resolves to %q, but that file does not exist or is not readable", modelID, modelPath)
	}

	return modelPath, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
