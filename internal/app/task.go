package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	internalagent "github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/config"
)

type TaskRequest struct {
	Message        string
	Provider       string
	Model          string
	PromptOverride string
	WorkingDir     string
	Allow          []string
	Device         string
	Config         config.Config
}

type TaskResult struct {
	Request         string
	Response        string
	RawOutput       string
	RawOutputTool   string
	DirectRawOutput bool
}

func (s *service) Task(ctx context.Context, req TaskRequest) (TaskResult, error) {
	taskRootDir, err := resolveTaskRootDir(req)
	if err != nil {
		return TaskResult{}, err
	}

	runtimeConfig := req.Config
	if runtimeConfig != nil {
		runtimeConfig = config.WithWorkingDir(runtimeConfig, taskRootDir)
	}

	runtime, err := NewRuntime(RuntimeRequest{
		Provider:   req.Provider,
		Model:      req.Model,
		WorkingDir: taskRootDir,
		Config:     runtimeConfig,
	})
	if err != nil {
		return TaskResult{}, err
	}

	taskPrompt, err := runtime.ResolveTaskPrompt(req.PromptOverride)
	if err != nil {
		return TaskResult{}, err
	}

	agentInstance := runtime.NewAgent(PromptSet{Task: taskPrompt})
	agentInstance.SetDevice(req.Device)
	response, err := agentInstance.TaskWithOptionsResult(ctx, req.Message, internalagent.TaskOptions{
		Allow: req.Allow,
		Dirs: internalagent.TaskDirs{
			RootDir:    taskRootDir,
			CurrentDir: taskRootDir,
		},
	})
	if err != nil {
		return TaskResult{}, err
	}

	return TaskResult{
		Request:         req.Message,
		Response:        response.DisplayText(),
		RawOutput:       response.RawOutput,
		RawOutputTool:   response.RawOutputTool,
		DirectRawOutput: response.DirectRawOutput,
	}, nil
}

func resolveTaskRootDir(req TaskRequest) (string, error) {
	if workingDir := strings.TrimSpace(req.WorkingDir); workingDir != "" {
		return filepath.Abs(workingDir)
	}

	if req.Config != nil {
		if configuredDir := strings.TrimSpace(req.Config.GetConfiguredWorkingDir()); configuredDir != "" {
			return filepath.Abs(configuredDir)
		}
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to resolve task working directory: %w", err)
	}

	return filepath.Abs(workingDir)
}
