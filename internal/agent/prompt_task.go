package agent

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/laszukdawid/terminal-agent/internal/connector"
)

const taskPromptTemplateText = `Original task: {{.OriginalQuery}}

Current phase: {{.Phase}}
Turn: {{.Iterations}} of {{.MaxTurns}}
Tool calls: {{.ToolCalls}} of {{.MaxToolCalls}}
Task root directory: {{.RootDir}}
Current working directory: {{.CurrentDir}}{{.History}}

What should I do next to complete this task? Should I use one of provided tools? Is the task finished? If so, provide the final answer.`

const taskFinalSummaryTemplateText = `I've been working on this task: {{.OriginalQuery}}

Task root directory: {{.RootDir}}
Current working directory: {{.CurrentDir}}{{.History}}

I've reached the task budget limit. Based on the above, provide a comprehensive final answer.`

var (
	taskPromptTemplate       = template.Must(template.New("task_prompt").Parse(taskPromptTemplateText))
	taskFinalSummaryTemplate = template.Must(template.New("task_final_summary").Parse(taskFinalSummaryTemplateText))
)

type taskPromptTemplateData struct {
	OriginalQuery string
	Phase         TaskPhase
	Iterations    int
	MaxTurns      int
	ToolCalls     int
	MaxToolCalls  int
	RootDir       string
	CurrentDir    string
	History       string
}

type taskFinalSummaryTemplateData struct {
	OriginalQuery string
	RootDir       string
	CurrentDir    string
	History       string
}

func buildTaskPrompt(state *TaskState) string {
	return strings.TrimSpace(renderTaskTemplate(taskPromptTemplate, taskPromptTemplateData{
		OriginalQuery: state.OriginalQuery,
		Phase:         state.Phase,
		Iterations:    state.Iterations,
		MaxTurns:      state.MaxTurns,
		ToolCalls:     state.ToolCalls,
		MaxToolCalls:  state.MaxIterations,
		RootDir:       state.Dirs.RootDir,
		CurrentDir:    state.Dirs.CurrentDir,
		History:       formatTaskPromptHistory(renderTaskHistoryForPrompt(state.Steps)),
	}))
}

func (a *Agent) finalizeSummary(ctx context.Context, state *TaskState) (string, error) {
	summary := renderTaskTemplate(taskFinalSummaryTemplate, taskFinalSummaryTemplateData{
		OriginalQuery: state.OriginalQuery,
		RootDir:       state.Dirs.RootDir,
		CurrentDir:    state.Dirs.CurrentDir,
		History:       formatTaskPromptHistory(renderTaskHistoryForSummary(state.Steps)),
	})

	qParams := connector.QueryParams{
		UserPrompt: StringPtr(summary),
		SysPrompt:  a.systemPromptTask,
		MaxTokens:  a.maxTokens * 2,
		Device:     a.device,
	}

	return a.Connector.Query(ctx, &qParams)
}

func renderTaskTemplate(tmpl *template.Template, data any) string {
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		panic(fmt.Sprintf("failed to render task prompt template: %v", err))
	}
	return out.String()
}

func formatTaskPromptHistory(history string) string {
	if history == "" {
		return ""
	}
	return "\n\nOrdered step history:\n" + history
}

func StringPtr(s string) *string {
	return &s
}

func TruncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "... [Truncated]"
	}
	return s
}
