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
	Request         string
	Response        string
	RawOutput       string
	RawOutputTool   string
	DirectRawOutput bool
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

	taskPrompt, err := runtime.ResolveTaskPrompt(req.PromptOverride)
	if err != nil {
		return TaskResult{}, err
	}

	agentInstance := runtime.NewAgent(PromptSet{Task: taskPrompt})
	response, err := agentInstance.TaskWithOptionsResult(ctx, req.Message, internalagent.TaskOptions{Allow: req.Allow})
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
