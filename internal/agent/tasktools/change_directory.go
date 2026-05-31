package tasktools

import (
	"fmt"
	"strings"
)

type changeDirectoryTool struct {
	name        string
	description string
	inputSchema map[string]any
}

func NewChangeDirectory(name string) *changeDirectoryTool {
	return &changeDirectoryTool{
		name:        name,
		description: "Change the task's current working directory. Use this before running tools that should operate from a different subdirectory.",
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]string{
					"type":        "string",
					"description": "Directory path relative to the current working directory, or absolute within the task root.",
				},
			},
			"required": []string{"path"},
		},
	}
}

func (t *changeDirectoryTool) Name() string {
	return t.name
}

func (t *changeDirectoryTool) Description() string {
	return t.description
}

func (t *changeDirectoryTool) InputSchema() map[string]any {
	return t.inputSchema
}

func (t *changeDirectoryTool) HelpText() string {
	return fmt.Sprintf("Help for %s: %s\n\n%v", t.Name(), t.Description(), t.InputSchema())
}

func (t *changeDirectoryTool) RunSchema(input map[string]any) (string, error) {
	path, ok := input["path"].(string)
	if !ok || strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("failed to extract path from tool input")
	}
	return path, nil
}

func (t *changeDirectoryTool) Run(input *string) (string, error) {
	return t.RunSchema(map[string]any{"path": *input})
}
