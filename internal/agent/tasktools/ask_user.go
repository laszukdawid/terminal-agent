package tasktools

import (
	"fmt"

	"github.com/laszukdawid/terminal-agent/internal/tools"
)

type askUserTool struct {
	name        string
	description string
	inputSchema map[string]any
	clarify     func(string) (string, error)
}

func NewAskUser(name string, clarify func(string) (string, error)) tools.Tool {
	return &askUserTool{
		name:        name,
		description: "Ask the user for clarification or additional information. This tool is used when the model needs more context to proceed with the task.",
		clarify:     clarify,
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]string{
					"type":        "string",
					"description": "Ask a question to the user to get more info required to solve or clarify their problem",
				},
			},
			"required": []string{"question"},
		},
	}
}

func (t *askUserTool) Name() string {
	return t.name
}

func (t *askUserTool) PermissionCategory() tools.PermissionCategory {
	return tools.PermissionRead
}

func (t *askUserTool) Description() string {
	return t.description
}

func (t *askUserTool) InputSchema() map[string]any {
	return t.inputSchema
}

func (t *askUserTool) HelpText() string {
	return fmt.Sprintf("Help for %s: %s\n\n%s", t.Name(), t.Description(), t.InputSchema())
}

func (t *askUserTool) RunSchema(input map[string]any) (string, error) {
	question, ok := input["question"].(string)
	if !ok {
		return "", fmt.Errorf("failed to extract question from tool input")
	}
	if t.clarify == nil {
		return "", fmt.Errorf("task interaction required")
	}
	return t.clarify(question)
}

func (t *askUserTool) Run(input *string) (string, error) {
	return t.RunSchema(map[string]any{"question": *input})
}
