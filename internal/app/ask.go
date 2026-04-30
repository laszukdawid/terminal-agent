package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/config"
)

type AskRequest struct {
	Message              string
	Provider             string
	Model                string
	PromptOverride       string
	UseMemory            bool
	MemoryPath           string
	WorkingDir           string
	ContextFiles         []string
	TerminalContextCount int
	Stream               bool
	Config               config.Config
}

type AskResult struct {
	Question string
	Response string
}

func (s *service) Ask(ctx context.Context, req AskRequest) (AskResult, error) {
	runtime, err := NewRuntime(RuntimeRequest{
		Provider:   req.Provider,
		Model:      req.Model,
		WorkingDir: req.WorkingDir,
		Config:     req.Config,
	})
	if err != nil {
		return AskResult{}, err
	}

	prompts, err := runtime.ResolvePrompts(PromptOptions{
		AskOverride: req.PromptOverride,
		UseMemory:   req.UseMemory,
		MemoryPath:  req.MemoryPath,
	})
	if err != nil {
		return AskResult{}, err
	}

	userQuestion := req.Message

	var contextParts []string
	if len(req.ContextFiles) > 0 {
		contextContent, err := BuildContextFromFiles(req.ContextFiles)
		if err != nil {
			return AskResult{}, fmt.Errorf("failed to read context files: %w", err)
		}
		if contextContent != "" {
			contextParts = append(contextParts, contextContent)
		}
	}

	if req.TerminalContextCount > 0 {
		terminalContext, err := BuildContextFromTerminal(req.TerminalContextCount)
		if err != nil {
			return AskResult{}, err
		}
		contextParts = append(contextParts, terminalContext)
	}

	if len(contextParts) > 0 {
		userQuestion = strings.Join(contextParts, "\n\n") + "\n\n" + userQuestion
	}

	agentInstance := runtime.NewAgent(prompts)

	response, err := agentInstance.Question(ctx, userQuestion, req.Stream)
	if err != nil {
		return AskResult{}, err
	}

	return AskResult{Question: userQuestion, Response: response}, nil
}
