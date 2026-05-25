package app

import (
	"context"
	"fmt"
	"strings"

	internalagent "github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
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
	Device               string
	Config               config.Config
}

type AskResult struct {
	Question string
	Response string
}

func (s *service) Ask(ctx context.Context, req AskRequest) (AskResult, error) {
	events, err := s.AskEvents(ctx, req)
	if err != nil {
		return AskResult{}, err
	}

	result := AskResult{}
	for event := range events {
		switch event.Type {
		case EventOutputDelta:
			result.Response += event.Text
		case EventCompleted:
			result.Question = event.Status
			result.Response = event.FinalOutput
		case EventFailed:
			return AskResult{}, event.Err
		}
	}

	return result, nil
}

func (s *service) AskEvents(ctx context.Context, req AskRequest) (<-chan Event, error) {
	runtime, prompts, userQuestion, err := prepareAsk(req)
	if err != nil {
		return nil, err
	}

	agentInstance := runtime.NewAgent(prompts)
	agentInstance.SetDevice(req.Device)
	events := make(chan Event)

	go func() {
		defer close(events)

		if err := emitEvent(ctx, events, newEvent(RunKindAsk, EventStarted)); err != nil {
			return
		}

		qParams := connector.QueryParams{
			UserPrompt: &userQuestion,
			SysPrompt:  &prompts.Ask,
			Stream:     req.Stream,
			MaxTokens:  req.Config.GetMaxTokens(),
			Device:     req.Device,
		}

		if req.Stream {
			qParams.OnStream = func(chunk string) error {
				event := newEvent(RunKindAsk, EventOutputDelta)
				event.Text = chunk
				return emitEvent(ctx, events, event)
			}
		}

		response, err := agentInstance.Connector.Query(ctx, &qParams)
		if err != nil {
			failed := newEvent(RunKindAsk, EventFailed)
			failed.Err = err
			_ = emitEvent(ctx, events, failed)
			return
		}

		completed := newEvent(RunKindAsk, EventCompleted)
		completed.Status = userQuestion
		completed.FinalOutput = response
		_ = emitEvent(ctx, events, completed)
	}()

	return events, nil
}

func prepareAsk(req AskRequest) (*Runtime, PromptSet, string, error) {
	if strings.TrimSpace(req.Message) == "" {
		return nil, PromptSet{}, "", internalagent.ErrEmptyQuery
	}

	runtime, err := NewRuntime(RuntimeRequest{
		Provider:   req.Provider,
		Model:      req.Model,
		WorkingDir: req.WorkingDir,
		Config:     req.Config,
	})
	if err != nil {
		return nil, PromptSet{}, "", err
	}

	prompts, err := runtime.ResolvePrompts(PromptOptions{
		AskOverride: req.PromptOverride,
		UseMemory:   req.UseMemory,
		MemoryPath:  req.MemoryPath,
	})
	if err != nil {
		return nil, PromptSet{}, "", err
	}

	userQuestion := req.Message

	var contextParts []string
	if len(req.ContextFiles) > 0 {
		contextContent, err := BuildContextFromFiles(req.ContextFiles)
		if err != nil {
			return nil, PromptSet{}, "", fmt.Errorf("failed to read context files: %w", err)
		}
		if contextContent != "" {
			contextParts = append(contextParts, contextContent)
		}
	}

	if req.TerminalContextCount > 0 {
		terminalContext, err := BuildContextFromTerminal(req.TerminalContextCount)
		if err != nil {
			return nil, PromptSet{}, "", err
		}
		contextParts = append(contextParts, terminalContext)
	}

	if len(contextParts) > 0 {
		userQuestion = strings.Join(contextParts, "\n\n") + "\n\n" + userQuestion
	}

	return runtime, prompts, userQuestion, nil
}
