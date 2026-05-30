package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	internalagent "github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/sessionlog"
)

type TaskRequest struct {
	Message        string
	Provider       string
	Model          string
	PromptOverride string
	WorkingDir     string
	Allow          []string
	Device         string
	// Timeout bounds the whole task run. 0 means no task-level timeout (unlimited).
	Timeout time.Duration
	Config  config.Config
}

// formatTaskTimeout renders a task timeout for the session log meta header.
// A zero duration is reported as "unlimited".
func formatTaskTimeout(d time.Duration) string {
	if d <= 0 {
		return "unlimited"
	}
	return d.String()
}

type TaskResult struct {
	Request         string
	Response        string
	RawOutput       string
	RawOutputTool   string
	DirectRawOutput bool
}

func (s *service) TaskEvents(ctx context.Context, req TaskRequest) (<-chan Event, error) {
	if strings.TrimSpace(req.Message) == "" {
		return nil, internalagent.ErrEmptyQuery
	}

	events := make(chan Event)
	meta := buildMeta("task", req.Provider, req.Model, req.WorkingDir, req.Message)
	meta.TaskTimeout = formatTaskTimeout(req.Timeout)
	recorder := sessionlog.New(SessionDir(), meta)
	interaction := &taskEventInteraction{ctx: ctx, events: events}

	go s.runTaskEvents(ctx, req, interaction, recorder, events)

	return events, nil
}

func (s *service) runTaskEvents(ctx context.Context, req TaskRequest, interaction *taskEventInteraction, recorder *sessionlog.Recorder, events chan Event) {
	defer close(events)

	recorder.Write(sessionlog.Record{Type: sessionlog.RecordRequest, Kind: string(RunKindTask), Text: req.Message})

	if err := emitEvent(ctx, events, newEvent(RunKindTask, EventStarted)); err != nil {
		return
	}

	onStep := func(step internalagent.TaskStep) {
		recorder.Write(taskStepToRecord(step))
	}

	result, err := executeTask(ctx, req, interaction, onStep)
	if err != nil {
		failed := newEvent(RunKindTask, EventFailed)
		failed.Err = err
		recorder.Write(sessionlog.Record{Type: sessionlog.RecordFailed, Kind: string(RunKindTask), Error: err.Error()})
		_ = emitEvent(ctx, events, failed)
		return
	}

	completed := newEvent(RunKindTask, EventCompleted)
	completed.Status = result.Request
	completed.FinalOutput = result.Response
	completed.RawOutput = result.RawOutput
	completed.RawOutputTool = result.RawOutputTool
	completed.DirectRawOutput = result.DirectRawOutput
	recorder.Write(sessionlog.Record{Type: sessionlog.RecordCompleted, Kind: string(RunKindTask), Text: result.Response, ToolName: result.RawOutputTool})
	_ = emitEvent(ctx, events, completed)
}

func executeTask(ctx context.Context, req TaskRequest, interaction internalagent.TaskInteraction, onStep func(internalagent.TaskStep)) (TaskResult, error) {
	if strings.TrimSpace(req.Message) == "" {
		return TaskResult{}, internalagent.ErrEmptyQuery
	}

	taskRootDir, err := resolveTaskRootDir(req)
	if err != nil {
		return TaskResult{}, err
	}

	runtimeConfig := req.Config
	if runtimeConfig != nil {
		runtimeConfig = config.WithWorkingDir(runtimeConfig, taskRootDir)
	}

	runtime, err := NewRuntime(RuntimeRequest{
		Provider:   req.Provider,
		Model:      req.Model,
		WorkingDir: taskRootDir,
		Config:     runtimeConfig,
	})
	if err != nil {
		return TaskResult{}, err
	}

	taskPrompt, err := runtime.ResolveTaskPrompt(req.PromptOverride)
	if err != nil {
		return TaskResult{}, err
	}

	agentInstance := runtime.NewAgent(PromptSet{Task: taskPrompt})
	agentInstance.SetDevice(req.Device)
	response, err := agentInstance.TaskWithOptionsResult(ctx, req.Message, internalagent.TaskOptions{
		Allow:       req.Allow,
		Interaction: interaction,
		OnStep:      onStep,
		Timeout:     req.Timeout,
		Dirs: internalagent.TaskDirs{
			RootDir:    taskRootDir,
			CurrentDir: taskRootDir,
		},
	})
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

var errTaskEventAlreadyReplied = errors.New("task event already replied")

type taskEventInteraction struct {
	ctx    context.Context
	events chan<- Event
}

func taskStepToRecord(step internalagent.TaskStep) sessionlog.Record {
	rec := sessionlog.Record{
		Kind:      string(RunKindTask),
		Timestamp: step.Timestamp,
		Iteration: step.Iteration,
		Text:      step.Thought,
		ToolName:  step.ToolName,
		ToolInput: step.ToolInput,
		Error:     step.Error,
	}

	switch step.Status {
	case internalagent.TaskStepStatusThought:
		rec.Type = sessionlog.RecordThought
	case internalagent.TaskStepStatusSucceeded:
		rec.Type = sessionlog.RecordToolResult
		rec.ToolResult = step.ToolOutput
	case internalagent.TaskStepStatusFailed:
		rec.Type = sessionlog.RecordToolCall
	case internalagent.TaskStepStatusDeclined:
		rec.Type = sessionlog.RecordDeclined
	case internalagent.TaskStepStatusFinalAnswer:
		// The run's authoritative completion is the app-level completed event; record the
		// final-answer step as the final_answer tool's result to avoid a duplicate line.
		rec.Type = sessionlog.RecordToolResult
		rec.ToolResult = step.FinalAnswer
	default:
		rec.Type = sessionlog.RecordType(step.Status)
	}

	return rec
}

func (i *taskEventInteraction) Confirm(req internalagent.TaskConfirmationRequest) (internalagent.TaskConfirmationDecision, error) {
	replies := make(chan TaskConfirmationResponse, 1)
	var once sync.Once

	event := newEvent(RunKindTask, EventConfirmationNeeded)
	event.Confirmation = &TaskConfirmationEvent{
		Action: req.Action,
		Reply: func(response TaskConfirmationResponse) error {
			var err error = errTaskEventAlreadyReplied
			once.Do(func() {
				err = nil
				select {
				case replies <- response:
				case <-i.ctx.Done():
					err = i.ctx.Err()
				}
			})
			return err
		},
	}

	if err := emitEvent(i.ctx, i.events, event); err != nil {
		return internalagent.TaskConfirmationDecision{}, err
	}

	select {
	case response := <-replies:
		return internalagent.TaskConfirmationDecision{Allowed: response.Allowed, Remember: response.Remember}, nil
	case <-i.ctx.Done():
		return internalagent.TaskConfirmationDecision{}, i.ctx.Err()
	}
}

func (i *taskEventInteraction) Clarify(req internalagent.TaskClarificationRequest) (string, error) {
	replies := make(chan string, 1)
	var once sync.Once

	event := newEvent(RunKindTask, EventClarificationNeeded)
	event.Clarification = &TaskClarificationEvent{
		Question: req.Question,
		Reply: func(response string) error {
			var err error = errTaskEventAlreadyReplied
			once.Do(func() {
				err = nil
				select {
				case replies <- response:
				case <-i.ctx.Done():
					err = i.ctx.Err()
				}
			})
			return err
		},
	}

	if err := emitEvent(i.ctx, i.events, event); err != nil {
		return "", err
	}

	select {
	case response := <-replies:
		return response, nil
	case <-i.ctx.Done():
		return "", i.ctx.Err()
	}
}

func resolveTaskRootDir(req TaskRequest) (string, error) {
	if workingDir := strings.TrimSpace(req.WorkingDir); workingDir != "" {
		return filepath.Abs(workingDir)
	}

	if req.Config != nil {
		if configuredDir := strings.TrimSpace(req.Config.GetConfiguredWorkingDir()); configuredDir != "" {
			return filepath.Abs(configuredDir)
		}
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to resolve task working directory: %w", err)
	}

	return filepath.Abs(workingDir)
}
