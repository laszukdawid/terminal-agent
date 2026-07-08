package agent

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	internalskills "github.com/laszukdawid/terminal-agent/internal/skills"
	"github.com/laszukdawid/terminal-agent/internal/taskmemory"
)

const taskPromptTemplateText = `Original task: {{.OriginalQuery}}

Current phase: {{.Phase}}
Turn: {{.Iterations}} of {{.MaxTurns}}
Tool calls: {{.ToolCalls}} of {{.MaxToolCalls}}
Task root directory: {{.RootDir}}
Current working directory: {{.CurrentDir}}
{{.RelevantMemories}}
{{.AvailableSkillsSection}}
{{.LoadedSkillsSection}}
{{.HistorySection}}

Decide internally whether another tool call is needed. If the task is complete, provide only the final answer requested by the user; do not mention that the task is complete unless the user asked about completion status.`

const taskFinalSummaryTemplateText = `I've been working on this task: {{.OriginalQuery}}

Task root directory: {{.RootDir}}
Current working directory: {{.CurrentDir}}
{{.HistorySection}}

I've reached the task budget limit. Based on the above, provide a comprehensive final answer.`

var (
	taskPromptTemplate       = template.Must(template.New("task_prompt").Parse(taskPromptTemplateText))
	taskFinalSummaryTemplate = template.Must(template.New("task_final_summary").Parse(taskFinalSummaryTemplateText))
)

type taskPromptTemplateData struct {
	OriginalQuery          string
	Phase                  TaskPhase
	Iterations             int
	MaxTurns               int
	ToolCalls              int
	MaxToolCalls           int
	RootDir                string
	CurrentDir             string
	RelevantMemories       string
	AvailableSkillsSection string
	LoadedSkillsSection    string
	HistorySection         string
}

type taskFinalSummaryTemplateData struct {
	OriginalQuery  string
	RootDir        string
	CurrentDir     string
	HistorySection string
}

func buildTaskPrompt(state *TaskState) (string, error) {
	rendered, err := renderTaskTemplate(taskPromptTemplate, taskPromptTemplateData{
		OriginalQuery:          state.OriginalQuery,
		Phase:                  state.Phase,
		Iterations:             state.Iterations,
		MaxTurns:               state.MaxTurns,
		ToolCalls:              state.ToolCalls,
		MaxToolCalls:           state.MaxIterations,
		RootDir:                state.Dirs.RootDir,
		CurrentDir:             state.Dirs.CurrentDir,
		RelevantMemories:       formatTaskPromptMemories(state.RelevantMemories),
		AvailableSkillsSection: formatTaskPromptSkills(state.Skills),
		LoadedSkillsSection:    formatTaskPromptLoadedSkills(state.LoadedSkills),
		HistorySection:         formatTaskPromptHistory(renderTaskHistoryForPrompt(state.Steps)),
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(rendered), nil
}

func (a *Agent) finalizeSummary(ctx context.Context, state *TaskState) (string, error) {
	summary, err := renderTaskTemplate(taskFinalSummaryTemplate, taskFinalSummaryTemplateData{
		OriginalQuery:  state.OriginalQuery,
		RootDir:        state.Dirs.RootDir,
		CurrentDir:     state.Dirs.CurrentDir,
		HistorySection: formatTaskPromptHistory(renderTaskHistoryForSummary(state.Steps)),
	})
	if err != nil {
		return "", err
	}
	qParams := connector.QueryParams{
		UserPrompt: StringPtr(summary),
		SysPrompt:  a.systemPromptTask,
		MaxTokens:  a.maxTokens * 2,
		Device:     a.device,
	}

	response, err := a.Connector.Query(ctx, &qParams)
	if err != nil {
		return "", err
	}
	state.TokensUsed += estimateTokens(summary, response)
	return response, nil
}

func renderTaskTemplate(tmpl *template.Template, data any) (string, error) {
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", fmt.Errorf("render task prompt template %q: %w", tmpl.Name(), err)
	}
	return out.String(), nil
}

func formatTaskPromptHistory(history string) string {
	if history == "" {
		return ""
	}
	return strings.TrimSpace("Ordered step history:\n" + history)
}

func formatTaskPromptSkills(skills []internalskills.Skill) string {
	if len(skills) == 0 {
		return ""
	}
	return strings.TrimSpace(internalskills.FormatIndex(skills))
}

func formatTaskPromptLoadedSkills(skills []internalskills.Skill) string {
	if len(skills) == 0 {
		return ""
	}
	return strings.TrimSpace(internalskills.FormatLoaded(skills))
}

func formatTaskPromptMemories(matches []taskmemory.Match) string {
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimSpace("Relevant prior task memories (advisory; reuse only when applicable):\n" + taskmemory.FormatMatches(matches))
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
