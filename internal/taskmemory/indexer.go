package taskmemory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	internaltools "github.com/laszukdawid/terminal-agent/internal/tools"
	log "github.com/laszukdawid/terminal-agent/internal/utils"
)

type TaskStep struct {
	Iteration   int
	Status      string
	Thought     string
	ToolName    string
	ToolInput   map[string]any
	ToolOutput  string
	Error       string
	FinalAnswer string
}

type AsyncIndexer struct {
	store Store
	env   Environment
	ch    chan Record
	done  chan struct{}
	once  sync.Once
}

func NewAsyncIndexer(store Store, env Environment) *AsyncIndexer {
	idx := &AsyncIndexer{store: store, env: env, ch: make(chan Record, 256), done: make(chan struct{})}
	go idx.run()
	return idx
}

func (i *AsyncIndexer) run() {
	defer close(i.done)
	for rec := range i.ch {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := i.store.Add(ctx, rec); err != nil {
			log.Warnw("failed to index task memory", "kind", rec.Kind, "error", err)
		}
		cancel()
	}
}

func (i *AsyncIndexer) Add(rec Record) {
	if i == nil || i.store == nil {
		return
	}
	rec.OS = firstNonEmpty(rec.OS, i.env.OS)
	rec.Arch = firstNonEmpty(rec.Arch, i.env.Arch)
	rec.Shell = firstNonEmpty(rec.Shell, i.env.Shell)
	rec.CWD = firstNonEmpty(rec.CWD, i.env.CWD)
	select {
	case i.ch <- rec:
	default:
		log.Warnw("dropping task memory record because index queue is full", "kind", rec.Kind)
	}
}

func (i *AsyncIndexer) Close(wait time.Duration) {
	if i == nil {
		return
	}
	i.once.Do(func() { close(i.ch) })
	if wait <= 0 {
		return
	}
	select {
	case <-i.done:
	case <-time.After(wait):
		log.Warnw("task memory indexing did not finish before timeout", "timeout", wait.String())
	}
}

func RequestRecord(runID string, request string, env Environment) Record {
	return Record{
		Kind:        KindTaskRequest,
		SourceRunID: runID,
		SourceKind:  "task",
		OS:          env.OS,
		Arch:        env.Arch,
		Shell:       env.Shell,
		CWD:         env.CWD,
		SearchableText: BuildSearchableText(
			"Kind: task_request",
			"User request: "+request,
			"OS: "+env.OS,
			"CWD: "+env.CWD,
		),
	}
}

func StepRecord(runID string, request string, step TaskStep, env Environment) (Record, bool) {
	status := strings.TrimSpace(step.Status)
	if status == "" || status == "thought" || status == "declined" {
		return Record{}, false
	}
	kind := KindToolFailure
	output := step.ToolOutput
	if status == "succeeded" {
		kind = KindToolSuccess
	} else if status == "final_answer" {
		kind = KindTaskCompletion
		output = step.FinalAnswer
	}
	if strings.TrimSpace(step.ToolName) == "" && strings.TrimSpace(output) == "" && strings.TrimSpace(step.Error) == "" {
		return Record{}, false
	}
	return Record{
		Kind:        kind,
		SourceRunID: runID,
		SourceSeq:   step.Iteration,
		SourceKind:  "task",
		OS:          env.OS,
		Arch:        env.Arch,
		Shell:       env.Shell,
		CWD:         env.CWD,
		ToolName:    step.ToolName,
		ToolInput:   step.ToolInput,
		FinalOutput: output,
		SearchableText: BuildSearchableText(
			"Kind: "+kind,
			"User request: "+request,
			"Thought: "+step.Thought,
			"Tool: "+step.ToolName,
			"Input: "+ToolInputJSON(step.ToolInput),
			"Output: "+output,
			"Error: "+step.Error,
			"OS: "+env.OS,
			"CWD: "+env.CWD,
		),
	}, true
}

func CompletionRecord(runID string, request string, response string, rawTool string, rawOutput string, env Environment) Record {
	return Record{
		Kind:        KindTaskCompletion,
		SourceRunID: runID,
		SourceKind:  "task",
		OS:          env.OS,
		Arch:        env.Arch,
		Shell:       env.Shell,
		CWD:         env.CWD,
		ToolName:    rawTool,
		FinalOutput: firstNonEmpty(response, rawOutput),
		SearchableText: BuildSearchableText(
			"Kind: task_completion",
			"User request: "+request,
			"Final answer: "+response,
			"Raw output: "+rawOutput,
			"Tool: "+rawTool,
			"OS: "+env.OS,
			"CWD: "+env.CWD,
		),
	}
}

func QueryFromTaskState(originalQuery string, latest TaskStep, env Environment) Query {
	text := BuildSearchableText(
		originalQuery,
		latest.Thought,
		latest.ToolName,
		ToolInputJSON(latest.ToolInput),
		latest.ToolOutput,
		latest.Error,
		latest.FinalAnswer,
	)
	return Query{Text: text, OS: env.OS, Arch: env.Arch, Shell: env.Shell, CWD: env.CWD, Limit: defaultSearchLimit, IncludeCWD: true}
}

func LatestStep(steps []TaskStep) TaskStep {
	if len(steps) == 0 {
		return TaskStep{}
	}
	return steps[len(steps)-1]
}

func ToolNameFromInput(toolName string, input map[string]any) string {
	if toolName != internaltools.ToolNameUnix {
		return toolName
	}
	cmd, _ := input["command"].(string)
	if cmd == "" {
		return toolName
	}
	return fmt.Sprintf("%s: %s", toolName, cmd)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
