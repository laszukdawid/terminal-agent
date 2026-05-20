package app

import (
	"fmt"

	internalagent "github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
)

type RuntimeRequest struct {
	Provider   string
	Model      string
	WorkingDir string
	Config     config.Config
}

type Runtime struct {
	Config       config.Config
	Connector    connector.LLMConnector
	ToolProvider tools.ToolProvider
	WorkingDir   string
}

type PromptSet struct {
	Ask  string
	Task string
}

type PromptOptions struct {
	AskOverride  string
	TaskOverride string
	UseMemory    bool
	MemoryPath   string
}

func NewRuntime(req RuntimeRequest) (*Runtime, error) {
	workingDir := req.WorkingDir
	if workingDir == "" && req.Config != nil {
		workingDir = req.Config.GetWorkingDir()
	}

	runtimeConfig := req.Config
	if runtimeConfig != nil && workingDir != "" {
		runtimeConfig = config.WithWorkingDir(runtimeConfig, workingDir)
	}

	conn, err := connector.NewConnector(req.Provider, req.Model, runtimeConfig)
	if err != nil {
		return nil, err
	}

	return &Runtime{
		Config:       runtimeConfig,
		Connector:    conn,
		ToolProvider: tools.NewToolProvider(runtimeConfig),
		WorkingDir:   workingDir,
	}, nil
}

func (r *Runtime) ResolvePrompts(opts PromptOptions) (PromptSet, error) {
	askPrompt, err := r.ResolveAskPrompt(opts)
	if err != nil {
		return PromptSet{}, err
	}

	taskPrompt, err := r.ResolveTaskPrompt(opts.TaskOverride)
	if err != nil {
		return PromptSet{}, err
	}

	return PromptSet{Ask: askPrompt, Task: taskPrompt}, nil
}

func (r *Runtime) ResolveAskPrompt(opts PromptOptions) (string, error) {
	askPrompt, err := internalagent.ResolvePrompt(opts.AskOverride, "ask", r.WorkingDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve ask prompt: %w", err)
	}

	askPrompt, err = BuildAskPrompt(askPrompt, opts.UseMemory, opts.MemoryPath)
	if err != nil {
		return "", err
	}

	return askPrompt, nil
}

func (r *Runtime) ResolveTaskPrompt(taskOverride string) (string, error) {
	taskPrompt, err := internalagent.ResolvePrompt(taskOverride, "task", r.WorkingDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve task prompt: %w", err)
	}

	return taskPrompt, nil
}

func (r *Runtime) NewAgent(prompts PromptSet) *internalagent.Agent {
	return internalagent.NewAgent(r.Connector, r.ToolProvider, r.Config, prompts.Ask, prompts.Task)
}
