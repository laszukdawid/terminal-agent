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
	store  Store
	env    Environment
	ch     chan Record
	done   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
}

func NewAsyncIndexer(store Store, env Environment) *AsyncIndexer {
	ctx, cancel := context.WithCancel(context.Background())
	idx := &AsyncIndexer{store: store, env: env, ch: make(chan Record, 256), done: make(chan struct{}), ctx: ctx, cancel: cancel}
	go idx.run()
	return idx
}

func (i *AsyncIndexer) run() {
	defer close(i.done)
	for rec := range i.ch {
		// Derive each per-record timeout from the indexer context so Close can
		// cancel an in-flight (and all remaining) Add at shutdown, letting the
		// loop exit promptly instead of the caller closing the store underneath it.
		ctx, cancel := context.WithTimeout(i.ctx, time.Second)
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

// Close stops accepting records and waits for the run loop to exit before
// returning, so a caller can safely close the store afterward without racing an
// in-flight Add. It first lets the queue drain for up to wait; if that deadline
// passes it cancels the indexer context (aborting the in-flight and remaining
// Adds) and then joins the loop, which by then exits promptly. Records still
// queued at the deadline are dropped by design, but the store is never closed
// while an Add is running.
func (i *AsyncIndexer) Close(wait time.Duration) {
	if i == nil {
		return
	}
	i.once.Do(func() { close(i.ch) })
	if wait > 0 {
		select {
		case <-i.done:
			i.cancel()
			return
		case <-time.After(wait):
			log.Warnw("task memory indexing did not finish before timeout; cancelling", "timeout", wait.String())
		}
	}
	i.cancel()
	<-i.done
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
	// A final_answer step is the run's completion, and CompletionRecord is the
	// single task_completion record per run; indexing the step too would store
	// the same answer twice and let near-duplicates crowd the retrieval slots.
	if status == "" || status == "thought" || status == "declined" || status == "final_answer" {
		return Record{}, false
	}
	kind := KindToolFailure
	output := step.ToolOutput
	if status == "succeeded" {
		kind = KindToolSuccess
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

// CompletionProvenance ties a run's completion record to the final answer
// step it replaces (StepRecord does not index final_answer steps): the model's
// closing thought and the step number. Completions without a final answer step
// — direct raw output and the max-turns summary fallback — pass the zero value.
type CompletionProvenance struct {
	Thought   string
	SourceSeq int
}

// CompletionRecord is the single task_completion record a completed run
// indexes. Its searchable text skips parts that would index the same words
// twice within the record: a thought that only restates the answer, and raw
// output identical to the response (the direct-output path).
func CompletionRecord(runID string, request string, response string, rawTool string, rawOutput string, prov CompletionProvenance, env Environment) Record {
	parts := []string{
		"Kind: task_completion",
		"User request: " + request,
	}
	if thought := strings.TrimSpace(prov.Thought); thought != "" && thought != strings.TrimSpace(response) {
		parts = append(parts, "Thought: "+prov.Thought)
	}
	parts = append(parts, "Final answer: "+response)
	if raw := strings.TrimSpace(rawOutput); raw != "" && raw != strings.TrimSpace(response) {
		parts = append(parts, "Raw output: "+rawOutput)
	}
	if rawTool != "" {
		parts = append(parts, "Tool: "+rawTool)
	}
	parts = append(parts, "OS: "+env.OS, "CWD: "+env.CWD)
	return Record{
		Kind:           KindTaskCompletion,
		SourceRunID:    runID,
		SourceSeq:      prov.SourceSeq,
		SourceKind:     "task",
		OS:             env.OS,
		Arch:           env.Arch,
		Shell:          env.Shell,
		CWD:            env.CWD,
		ToolName:       rawTool,
		FinalOutput:    firstNonEmpty(response, rawOutput),
		SearchableText: BuildSearchableText(parts...),
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
