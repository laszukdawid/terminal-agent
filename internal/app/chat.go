package app

import (
	"context"
	"fmt"
	"strings"

	internalagent "github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/chat"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
)

type ChatRequest struct {
	Message        string
	Provider       string
	Model          string
	PromptOverride string
	UseMemory      bool
	MemoryPath     string
	WorkingDir     string
	ContextFiles   []string
	Stream         bool
	NewSession     bool
	ChatDBPath     string
	Config         config.Config
}

type ChatResult struct {
	Message  string
	Response string
}

func (s *service) Chat(ctx context.Context, req ChatRequest) (ChatResult, error) {
	events, err := s.ChatEvents(ctx, req)
	if err != nil {
		return ChatResult{}, err
	}

	result := ChatResult{}
	for event := range events {
		switch event.Type {
		case EventOutputDelta:
			result.Response += event.Text
		case EventCompleted:
			result.Message = event.Status
			result.Response = event.FinalOutput
		case EventFailed:
			return ChatResult{}, event.Err
		}
	}

	return result, nil
}

func (s *service) ChatEvents(ctx context.Context, req ChatRequest) (<-chan Event, error) {
	runtime, prompts, connectorMessages, userMessage, sessionStore, err := prepareChat(req)
	if err != nil {
		return nil, err
	}

	agentInstance := runtime.NewAgent(prompts)
	if agentInstance.Connector == nil {
		sessionStore.Close()
		return nil, fmt.Errorf("failed to initialize %s connector", req.Provider)
	}
	events := make(chan Event)

	go func() {
		defer close(events)
		defer sessionStore.Close()

		_ = emitEvent(events, newEvent(RunKindChat, EventStarted))

		qParams := connector.QueryParams{
			UserPrompt: &userMessage,
			SysPrompt:  &prompts.Ask,
			Messages:   connectorMessages,
			Stream:     req.Stream,
			MaxTokens:  req.Config.GetMaxTokens(),
		}

		if req.Stream {
			qParams.OnStream = func(chunk string) error {
				event := newEvent(RunKindChat, EventOutputDelta)
				event.Text = chunk
				return emitEvent(events, event)
			}
		}

		response, err := agentInstance.Connector.Query(ctx, &qParams)
		if err != nil {
			failed := newEvent(RunKindChat, EventFailed)
			failed.Err = err
			_ = emitEvent(events, failed)
			return
		}

		if err := sessionStore.AddMessage("assistant", response); err != nil {
			failed := newEvent(RunKindChat, EventFailed)
			failed.Err = fmt.Errorf("failed to save assistant response: %w", err)
			_ = emitEvent(events, failed)
			return
		}

		completed := newEvent(RunKindChat, EventCompleted)
		completed.Status = strings.TrimSpace(userMessage)
		completed.FinalOutput = response
		_ = emitEvent(events, completed)
	}()

	return events, nil
}

func prepareChat(req ChatRequest) (*Runtime, PromptSet, []connector.Message, string, *chat.SessionStore, error) {
	if strings.TrimSpace(req.Message) == "" {
		return nil, PromptSet{}, nil, "", nil, internalagent.ErrEmptyQuery
	}

	runtime, err := NewRuntime(RuntimeRequest{
		Provider:   req.Provider,
		Model:      req.Model,
		WorkingDir: req.WorkingDir,
		Config:     req.Config,
	})
	if err != nil {
		return nil, PromptSet{}, nil, "", nil, err
	}

	prompts, err := runtime.ResolvePrompts(PromptOptions{
		AskOverride: req.PromptOverride,
		UseMemory:   req.UseMemory,
		MemoryPath:  req.MemoryPath,
	})
	if err != nil {
		return nil, PromptSet{}, nil, "", nil, err
	}

	sessionStore, err := chat.NewSessionStore(req.ChatDBPath)
	if err != nil {
		return nil, PromptSet{}, nil, "", nil, fmt.Errorf("failed to initialize chat session: %w", err)
	}

	if req.NewSession {
		_, err = sessionStore.NewSession()
		if err != nil {
			sessionStore.Close()
			return nil, PromptSet{}, nil, "", nil, fmt.Errorf("failed to create new session: %w", err)
		}
	}

	_, err = sessionStore.GetOrCreateSession()
	if err != nil {
		sessionStore.Close()
		return nil, PromptSet{}, nil, "", nil, fmt.Errorf("failed to get session: %w", err)
	}

	chatHistory, err := sessionStore.GetMessages()
	if err != nil {
		sessionStore.Close()
		return nil, PromptSet{}, nil, "", nil, fmt.Errorf("failed to load chat history: %w", err)
	}

	connectorMessages := make([]connector.Message, 0, len(chatHistory))
	for _, msg := range chatHistory {
		connectorMessages = append(connectorMessages, connector.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	userMessage := req.Message
	if len(req.ContextFiles) > 0 {
		contextContent, err := BuildContextFromFiles(req.ContextFiles)
		if err != nil {
			sessionStore.Close()
			return nil, PromptSet{}, nil, "", nil, fmt.Errorf("failed to read context files: %w", err)
		}
		userMessage = contextContent + "\n\n" + userMessage
	}

	if err := sessionStore.AddMessage("user", userMessage); err != nil {
		sessionStore.Close()
		return nil, PromptSet{}, nil, "", nil, fmt.Errorf("failed to save user message: %w", err)
	}

	return runtime, prompts, connectorMessages, userMessage, sessionStore, nil
}
