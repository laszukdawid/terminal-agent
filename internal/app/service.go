package app

import "context"

type Service interface {
	Ask(ctx context.Context, req AskRequest) (AskResult, error)
	Chat(ctx context.Context, req ChatRequest) (ChatResult, error)
	Task(ctx context.Context, req TaskRequest) (TaskResult, error)
}

type service struct{}

func NewService() Service {
	return &service{}
}
