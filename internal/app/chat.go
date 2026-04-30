package app

import (
	"context"
	"fmt"
	"strings"

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
	runtime, err := NewRuntime(RuntimeRequest{
		Provider:   req.Provider,
		Model:      req.Model,
		WorkingDir: req.WorkingDir,
		Config:     req.Config,
	})
	if err != nil {
		return ChatResult{}, err
	}

	prompts, err := runtime.ResolvePrompts(PromptOptions{
		AskOverride: req.PromptOverride,
		UseMemory:   req.UseMemory,
		MemoryPath:  req.MemoryPath,
	})
	if err != nil {
		return ChatResult{}, err
	}

	sessionStore, err := chat.NewSessionStore(req.ChatDBPath)
	if err != nil {
		return ChatResult{}, fmt.Errorf("failed to initialize chat session: %w", err)
	}
	defer sessionStore.Close()

	if req.NewSession {
		_, err = sessionStore.NewSession()
		if err != nil {
			return ChatResult{}, fmt.Errorf("failed to create new session: %w", err)
		}
	}

	_, err = sessionStore.GetOrCreateSession()
	if err != nil {
		return ChatResult{}, fmt.Errorf("failed to get session: %w", err)
	}

	chatHistory, err := sessionStore.GetMessages()
	if err != nil {
		return ChatResult{}, fmt.Errorf("failed to load chat history: %w", err)
	}

	var connectorMessages []connector.Message
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
			return ChatResult{}, fmt.Errorf("failed to read context files: %w", err)
		}
		userMessage = contextContent + "\n\n" + userMessage
	}

	if err := sessionStore.AddMessage("user", userMessage); err != nil {
		return ChatResult{}, fmt.Errorf("failed to save user message: %w", err)
	}

	agentInstance := runtime.NewAgent(prompts)
	response, err := agentInstance.Chat(ctx, userMessage, connectorMessages, req.Stream)
	if err != nil {
		return ChatResult{}, err
	}

	if err := sessionStore.AddMessage("assistant", response); err != nil {
		return ChatResult{}, fmt.Errorf("failed to save assistant response: %w", err)
	}

	return ChatResult{Message: strings.TrimSpace(userMessage), Response: response}, nil
}
