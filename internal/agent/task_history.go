package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/taskmemory"
)

type TaskStepStatus string

const (
	TaskStepStatusThought     TaskStepStatus = "thought"
	TaskStepStatusSucceeded   TaskStepStatus = "succeeded"
	TaskStepStatusFailed      TaskStepStatus = "failed"
	TaskStepStatusDeclined    TaskStepStatus = "declined"
	TaskStepStatusFinalAnswer TaskStepStatus = "final_answer"
)

type TaskStep struct {
	Iteration   int
	Timestamp   time.Time
	Status      TaskStepStatus
	Thought     string
	ToolName    string
	ToolInput   map[string]any
	ToolOutput  string
	Error       string
	Message     string
	FinalAnswer string
}

func (s *TaskState) appendStep(step TaskStep) {
	if step.Iteration == 0 {
		step.Iteration = s.Iterations
	}
	if step.Timestamp.IsZero() {
		step.Timestamp = time.Now().UTC()
	}
	s.Steps = append(s.Steps, step)
}

func taskLatestStep(steps []TaskStep) TaskStep {
	if len(steps) == 0 {
		return TaskStep{}
	}
	return steps[len(steps)-1]
}

func taskStepToMemoryStep(step TaskStep) taskmemory.TaskStep {
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

type taskHistoryRenderOptions struct {
	thoughtLimit     int
	inputLimit       int
	messageLimit     int
	errorLimit       int
	outputLimit      int
	finalAnswerLimit int
}

const (
	promptTaskHistoryOutputLimit       = 8000
	promptTaskHistoryFinalAnswerLimit  = 8000
	summaryTaskHistoryOutputLimit      = 12000
	summaryTaskHistoryFinalAnswerLimit = 12000
)

var promptTaskHistoryRenderOptions = taskHistoryRenderOptions{
	thoughtLimit:     600,
	inputLimit:       500,
	messageLimit:     500,
	errorLimit:       1000,
	outputLimit:      promptTaskHistoryOutputLimit,
	finalAnswerLimit: promptTaskHistoryFinalAnswerLimit,
}

var summaryTaskHistoryRenderOptions = taskHistoryRenderOptions{
	thoughtLimit:     1200,
	inputLimit:       1000,
	messageLimit:     1000,
	errorLimit:       1600,
	outputLimit:      summaryTaskHistoryOutputLimit,
	finalAnswerLimit: summaryTaskHistoryFinalAnswerLimit,
}

func renderTaskHistoryForPrompt(steps []TaskStep) string {
	return renderTaskHistory(steps, promptTaskHistoryRenderOptions)
}

func renderTaskHistoryForSummary(steps []TaskStep) string {
	return renderTaskHistory(steps, summaryTaskHistoryRenderOptions)
}

func renderTaskHistory(steps []TaskStep, opts taskHistoryRenderOptions) string {
	if len(steps) == 0 {
		return ""
	}

	sections := make([]string, 0, len(steps))
	for index, step := range steps {
		lines := []string{fmt.Sprintf("Step %d", index+1)}
		if step.Iteration > 0 {
			lines = append(lines, fmt.Sprintf("Iteration: %d", step.Iteration))
		}
		if !step.Timestamp.IsZero() {
			lines = append(lines, "Timestamp: "+step.Timestamp.UTC().Format(time.RFC3339))
		}
		if step.Status != "" {
			lines = append(lines, "Status: "+string(step.Status))
		}
		if thought := strings.TrimSpace(step.Thought); thought != "" {
			lines = append(lines, "Thought: "+TruncateString(thought, opts.thoughtLimit))
		}
		if step.ToolName != "" {
			lines = append(lines, "Tool: "+step.ToolName)
		}
		if len(step.ToolInput) > 0 {
			lines = append(lines, "Input: "+TruncateString(formatTaskToolInput(step.ToolInput), opts.inputLimit))
		}
		if message := strings.TrimSpace(step.Message); message != "" {
			lines = append(lines, "Message: "+TruncateString(message, opts.messageLimit))
		}
		if output := strings.TrimSpace(step.ToolOutput); output != "" {
			lines = append(lines, formatTaskBlock("OUTPUT", output, opts.outputLimit))
		}
		if finalAnswer := strings.TrimSpace(step.FinalAnswer); finalAnswer != "" {
			lines = append(lines, formatTaskBlock("FINAL_ANSWER", finalAnswer, opts.finalAnswerLimit))
		}
		if stepErr := strings.TrimSpace(step.Error); stepErr != "" {
			lines = append(lines, formatTaskBlock("ERROR", stepErr, opts.errorLimit))
		}

		sections = append(sections, strings.Join(lines, "\n"))
	}

	return strings.Join(sections, "\n\n")
}

func formatTaskToolInput(input map[string]any) string {
	if len(input) == 0 {
		return "{}"
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return fmt.Sprint(input)
	}
	return string(encoded)
}

func formatTaskBlock(label string, value string, maxLen int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return fmt.Sprintf("%s:\n<%s>\n%s\n</%s>", label, label, TruncateString(trimmed, maxLen), label)
}
