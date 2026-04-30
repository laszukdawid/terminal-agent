package app

import (
	"context"

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
	Config         config.Config
}

type TaskResult struct {
	Request  string
	Response string
}

func (s *service) Task(ctx context.Context, req TaskRequest) (TaskResult, error) {
	runtime, err := NewRuntime(RuntimeRequest{
		Provider:   req.Provider,
		Model:      req.Model,
		WorkingDir: req.WorkingDir,
		Config:     req.Config,
	})
	if err != nil {
		return TaskResult{}, err
	}

	prompts, err := runtime.ResolvePrompts(PromptOptions{
		TaskOverride: req.PromptOverride,
	})
	if err != nil {
		return TaskResult{}, err
	}

	agentInstance := runtime.NewAgent(prompts)
	response, err := agentInstance.TaskWithOptions(ctx, req.Message, internalagent.TaskOptions{Allow: req.Allow})
	if err != nil {
		return TaskResult{}, err
	}

	return TaskResult{Request: req.Message, Response: response}, nil
}
