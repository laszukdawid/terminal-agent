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
	TaskEvents(ctx context.Context, req TaskRequest) (<-chan Event, error)
}

type RunKind string

const (
	RunKindAsk  RunKind = "ask"
	RunKindChat RunKind = "chat"
	RunKindTask RunKind = "task"
)

type EventType string

const (
	EventStarted             EventType = "started"
	EventOutputDelta         EventType = "output_delta"
	EventConfirmationNeeded  EventType = "confirmation_needed"
	EventClarificationNeeded EventType = "clarification_needed"
	EventCompleted           EventType = "completed"
	EventFailed              EventType = "failed"
)

type TaskConfirmationResponse struct {
	Allowed  bool
	Remember bool
}

type TaskConfirmationEvent struct {
	Action string
	Reply  func(TaskConfirmationResponse) error
}

type TaskClarificationEvent struct {
	Question string
	Reply    func(string) error
}

type Event struct {
	Kind            RunKind
	Type            EventType
	Timestamp       time.Time
	Text            string
	Status          string
	FinalOutput     string
	Err             error
	ToolName        string
	ToolInput       map[string]any
	ToolResult      string
	RawOutput       string
	RawOutputTool   string
	DirectRawOutput bool
	Confirmation    *TaskConfirmationEvent
	Clarification   *TaskClarificationEvent
}

type service struct{}

func NewService() Service {
	return &service{}
}
