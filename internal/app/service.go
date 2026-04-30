package app

import (
	"context"
	"time"
)

type Service interface {
	Ask(ctx context.Context, req AskRequest) (AskResult, error)
	AskEvents(ctx context.Context, req AskRequest) (<-chan Event, error)
	Chat(ctx context.Context, req ChatRequest) (ChatResult, error)
	ChatEvents(ctx context.Context, req ChatRequest) (<-chan Event, error)
	Task(ctx context.Context, req TaskRequest) (TaskResult, error)
}

type RunKind string

const (
	RunKindAsk  RunKind = "ask"
	RunKindChat RunKind = "chat"
	RunKindTask RunKind = "task"
)

type EventType string

const (
	EventStarted            EventType = "started"
	EventOutputDelta        EventType = "output_delta"
	EventStatus             EventType = "status"
	EventToolCallRequested  EventType = "tool_call_requested"
	EventToolCallCompleted  EventType = "tool_call_completed"
	EventConfirmationNeeded EventType = "confirmation_needed"
	EventCompleted          EventType = "completed"
	EventFailed             EventType = "failed"
)

type Event struct {
	Kind        RunKind
	Type        EventType
	Timestamp   time.Time
	Text        string
	Status      string
	FinalOutput string
	Err         error
	ToolName    string
	ToolInput   map[string]any
	ToolResult  string
}

type service struct{}

func NewService() Service {
	return &service{}
}
