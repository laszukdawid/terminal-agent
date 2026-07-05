package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	internalagent "github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/sessionlog"
	"github.com/laszukdawid/terminal-agent/internal/taskmemory"
	log "github.com/laszukdawid/terminal-agent/internal/utils"
)

type TaskRequest struct {
	Message        string
	Provider       string
	Model          string
	PromptOverride string
	WorkingDir     string
	Allow          []string
	Deny           []string
	AutoApprove    bool
	Device         string
	// Timeout bounds the whole task run. 0 means no task-level timeout (unlimited).
	Timeout time.Duration
	// TokenBudget caps estimated tokens for the run; 0 means unlimited.
	TokenBudget int
	// MaxTurns and MaxToolCalls bound the step budget; 0 uses the agent defaults.
	MaxTurns     int
	MaxToolCalls int
	// EnabledTools selects which tools the run may use; nil exposes all available
	// tools (subject to DisableExternalTools).
	EnabledTools []string
	// DisableExternalTools drops external-facing tools (web search, MCP) when
	// EnabledTools is nil. Routines set this; interactive runs leave it false.
	DisableExternalTools bool
	Config               config.Config
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
	TokensUsed      int
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

	memoryStore, memoryIndexer, memoryEnv := openTaskMemory(req, taskRootEnv(req.WorkingDir), recorder.RunID())
	defer func() {
		if memoryIndexer != nil {
			memoryIndexer.Close(time.Second)
		}
		if memoryStore != nil {
			_ = memoryStore.Close()
		}
	}()
	if memoryIndexer != nil {
		memoryIndexer.Add(taskmemory.RequestRecord(recorder.RunID(), req.Message, memoryEnv))
	}

	onStep := func(step internalagent.TaskStep) {
		recorder.Write(taskStepToRecord(step))
		if memoryIndexer != nil {
			if rec, ok := taskmemory.StepRecord(recorder.RunID(), req.Message, taskStepToMemoryStep(step), memoryEnv); ok {
				memoryIndexer.Add(rec)
			}
		}
	}
	onStatus := func(status internalagent.TaskStatusEvent) {
		recorder.Write(taskStatusToRecord(status))
		event := newEvent(RunKindTask, EventTaskStatus)
		event.Timestamp = status.Timestamp
		event.Status = string(status.Phase)
		event.Text = status.Message
		event.ToolName = status.ToolName
		event.ToolInput = status.ToolInput
		_ = emitEvent(ctx, events, event)
	}
	onProgress := func(progress internalagent.TaskProgressEvent) {
		recorder.Write(taskProgressToRecord(progress))
		event := newEvent(RunKindTask, EventToolProgress)
		event.Timestamp = progress.Timestamp
		event.Text = progress.Message
		event.ToolName = progress.ToolName
		_ = emitEvent(ctx, events, event)
	}
	onMemory := func(memory internalagent.TaskMemoryEvent) {
		recorder.Write(taskMemoryToRecord(memory))
	}
	onToolOutput := func(output internalagent.TaskToolOutputEvent) error {
		if output.Err != nil {
			warning := newEvent(RunKindTask, EventWarning)
			warning.ToolName = output.ToolName
			warning.ProcessID = output.ProcessID
			warning.Text = formatToolOutputWarning(output.ProcessID, output.Err)
			if emitErr := emitEvent(ctx, events, warning); emitErr != nil {
				// If this channel is the failing display path, logs are the only reliable signal.
				log.Warnw("Failed to emit task output warning", "tool", output.ToolName, "process_id", output.ProcessID, "warning", warning.Text, "error", emitErr)
			}
			return nil
		}

		event := newEvent(RunKindTask, EventOutputDelta)
		event.Text = output.Output
		event.ToolName = output.ToolName
		event.ProcessID = output.ProcessID
		return emitEvent(ctx, events, event)
	}

	result, err := executeTask(ctx, req, interaction, onStep, onStatus, onProgress, onToolOutput, onMemory, memoryStore, memoryEnv)
	if err != nil {
		onStatus(internalagent.TaskStatusEvent{Phase: internalagent.TaskStatusFailed, Message: "Task failed.", Timestamp: time.Now().UTC()})
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
	if memoryIndexer != nil {
		memoryIndexer.Add(taskmemory.CompletionRecord(recorder.RunID(), req.Message, result.Response, result.RawOutputTool, result.RawOutput, memoryEnv))
	}
	_ = emitEvent(ctx, events, completed)
}

func executeTask(ctx context.Context, req TaskRequest, interaction internalagent.TaskInteraction, onStep func(internalagent.TaskStep), onStatus func(internalagent.TaskStatusEvent), onProgress func(internalagent.TaskProgressEvent), onToolOutput func(internalagent.TaskToolOutputEvent) error, onMemory func(internalagent.TaskMemoryEvent), memoryStore taskmemory.Store, memoryEnv taskmemory.Environment) (TaskResult, error) {
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
		Allow:                req.Allow,
		Deny:                 req.Deny,
		AutoApprove:          req.AutoApprove,
		Interaction:          interaction,
		OnStep:               onStep,
		OnStatus:             onStatus,
		OnProgress:           onProgress,
		OnToolOutput:         onToolOutput,
		OnMemory:             onMemory,
		Timeout:              req.Timeout,
		TokenBudget:          req.TokenBudget,
		MaxTurns:             req.MaxTurns,
		MaxToolCalls:         req.MaxToolCalls,
		EnabledTools:         req.EnabledTools,
		DisableExternalTools: req.DisableExternalTools,
		MemoryStore:          memoryStore,
		MemoryEnv:            memoryEnv,
		MemoryLimit:          5,
		Dirs: internalagent.TaskDirs{
			RootDir:    taskRootDir,
			CurrentDir: taskRootDir,
		},
	})
	if err != nil {
		return TaskResult{TokensUsed: response.TokensUsed}, err
	}

	return TaskResult{
		Request:         req.Message,
		Response:        response.DisplayText(),
		RawOutput:       response.RawOutput,
		RawOutputTool:   response.RawOutputTool,
		DirectRawOutput: response.DirectRawOutput,
		TokensUsed:      response.TokensUsed,
	}, nil
}

// taskStatusToRecord and the other converters leave Kind unset so the recorder
// stamps the run's kind (task or routine) from its meta header.

func openTaskMemory(req TaskRequest, env taskmemory.Environment, runID string) (taskmemory.Store, *taskmemory.AsyncIndexer, taskmemory.Environment) {
	if req.Config != nil && !req.Config.GetTaskMemory() {
		return nil, nil, env
	}
	store, err := taskmemory.Open(TaskMemoryPath())
	if err != nil {
		log.Warnw("failed to open task memory database", "run_id", runID, "error", err)
		return nil, nil, env
	}
	return store, taskmemory.NewAsyncIndexer(store, env), env
}

func taskRootEnv(workingDir string) taskmemory.Environment {
	cwd := strings.TrimSpace(workingDir)
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	return taskmemory.Environment{OS: runtime.GOOS, Arch: runtime.GOARCH, Shell: filepath.Base(os.Getenv("SHELL")), CWD: cwd}
}

func taskStepToMemoryStep(step internalagent.TaskStep) taskmemory.TaskStep {
	return taskmemory.TaskStep{
		Iteration:   step.Iteration,
		Status:      string(step.Status),
		Thought:     step.Thought,
		ToolName:    step.ToolName,
		ToolInput:   step.ToolInput,
		ToolOutput:  step.ToolOutput,
		Error:       step.Error,
		FinalAnswer: step.FinalAnswer,
	}
}

func taskStatusToRecord(status internalagent.TaskStatusEvent) sessionlog.Record {
	return sessionlog.Record{
		Type:      sessionlog.RecordProgress,
		Timestamp: status.Timestamp,
		Status:    string(status.Phase),
		Text:      status.Message,
		ToolName:  status.ToolName,
		ToolInput: status.ToolInput,
	}
}

func taskProgressToRecord(progress internalagent.TaskProgressEvent) sessionlog.Record {
	return sessionlog.Record{
		Type:      sessionlog.RecordProgress,
		Timestamp: progress.Timestamp,
		Text:      progress.Message,
		ToolName:  progress.ToolName,
	}
}

func taskMemoryToRecord(memory internalagent.TaskMemoryEvent) sessionlog.Record {
	text := fmt.Sprintf("Task memory retrieval: %d matches", len(memory.Matches))
	if memory.Err != nil {
		text = "Task memory retrieval failed"
	}
	return sessionlog.Record{
		Type:       sessionlog.RecordProgress,
		Timestamp:  memory.Timestamp,
		Status:     "task_memory_retrieval",
		Text:       text,
		ToolName:   internalagent.ToolNameMemorySearch,
		ToolInput:  map[string]any{"query": memory.Query, "limit": len(memory.Matches)},
		ToolResult: taskmemory.FormatMatches(memory.Matches),
		Error:      memoryError(memory.Err),
	}
}

func memoryError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func formatToolOutputWarning(processID int, err error) string {
	if processID > 0 {
		return fmt.Sprintf("Live output display failed; process %d is still running: %v", processID, err)
	}
	return fmt.Sprintf("Live output display failed; process is still running: %v", err)
}

var errTaskEventAlreadyReplied = errors.New("task event already replied")

type taskEventInteraction struct {
	ctx    context.Context
	events chan<- Event
}

func taskStepToRecord(step internalagent.TaskStep) sessionlog.Record {
	rec := sessionlog.Record{
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
		return internalagent.TaskConfirmationDecision{Allowed: response.Allowed, Remember: response.Remember, Patterns: response.Patterns}, nil
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
