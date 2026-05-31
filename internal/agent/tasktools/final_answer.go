package tasktools

import (
	"fmt"

	"github.com/laszukdawid/terminal-agent/internal/tools"
)

type finalAnswerTool struct {
	name        string
	description string
	inputSchema map[string]any
}

func NewFinalAnswer(name string) *finalAnswerTool {
	return &finalAnswerTool{
		name:        name,
		description: "Provide the final answer to the task.",
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"answer": map[string]string{
					"type":        "string",
					"description": "Call this tool when the task is complete and you want to provide the final answer.",
				},
			},
			"required": []string{"answer"},
		},
	}
}

func (t *finalAnswerTool) Name() string {
	return t.name
}

func (t *finalAnswerTool) PermissionCategory() tools.PermissionCategory {
	return tools.PermissionRead
}

func (t *finalAnswerTool) Description() string {
	return t.description
}

func (t *finalAnswerTool) InputSchema() map[string]any {
	return t.inputSchema
}

func (t *finalAnswerTool) HelpText() string {
	return fmt.Sprintf("Help for %s: %s\n\n%s", t.Name(), t.Description(), t.InputSchema())
}

func (t *finalAnswerTool) RunSchema(input map[string]any) (string, error) {
	answer, ok := input["answer"].(string)
	if !ok {
		return "", fmt.Errorf("failed to extract answer from tool input")
	}
	return answer, nil
}

func (t *finalAnswerTool) Run(input *string) (string, error) {
	return t.RunSchema(map[string]any{"answer": *input})
}
