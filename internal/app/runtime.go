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

	conn := connector.NewConnector(req.Provider, req.Model)

	return &Runtime{
		Config:       req.Config,
		Connector:    *conn,
		ToolProvider: tools.NewToolProvider(req.Config),
		WorkingDir:   workingDir,
	}, nil
}

func (r *Runtime) ResolvePrompts(opts PromptOptions) (PromptSet, error) {
	askPrompt, err := internalagent.ResolvePrompt(opts.AskOverride, "ask", r.WorkingDir)
	if err != nil {
		return PromptSet{}, fmt.Errorf("failed to resolve ask prompt: %w", err)
	}

	taskPrompt, err := internalagent.ResolvePrompt(opts.TaskOverride, "task", r.WorkingDir)
	if err != nil {
		return PromptSet{}, fmt.Errorf("failed to resolve task prompt: %w", err)
	}

	askPrompt, err = BuildAskPrompt(askPrompt, opts.UseMemory, opts.MemoryPath)
	if err != nil {
		return PromptSet{}, err
	}

	return PromptSet{Ask: askPrompt, Task: taskPrompt}, nil
}

func (r *Runtime) NewAgent(prompts PromptSet) *internalagent.Agent {
	return internalagent.NewAgent(r.Connector, r.ToolProvider, r.Config, prompts.Ask, prompts.Task)
}
