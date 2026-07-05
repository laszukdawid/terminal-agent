package tasktools

import (
	"fmt"

	internalskills "github.com/laszukdawid/terminal-agent/internal/skills"
	"github.com/laszukdawid/terminal-agent/internal/tools"
)

type SkillLoader interface {
	LoadSkill(name string) (internalskills.Skill, error)
}

type skillTool struct {
	name        string
	description string
	loader      SkillLoader
	inputSchema map[string]any
}

func NewSkill(name string, loader SkillLoader) *skillTool {
	return &skillTool{
		name:        name,
		description: "Load full instructions for an available skill by name.",
		loader:      loader,
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]string{
					"type":        "string",
					"description": "Skill name from the available skills index.",
				},
			},
			"required": []string{"name"},
		},
	}
}

func (t *skillTool) Name() string { return t.name }

func (t *skillTool) PermissionCategory() tools.PermissionCategory { return tools.PermissionRead }

func (t *skillTool) Description() string { return t.description }

func (t *skillTool) InputSchema() map[string]any { return t.inputSchema }

func (t *skillTool) HelpText() string {
	return fmt.Sprintf("Help for %s: %s\n\n%s", t.Name(), t.Description(), t.InputSchema())
}

func (t *skillTool) Run(input *string) (string, error) {
	return t.RunSchema(map[string]any{"name": *input})
}

func (t *skillTool) RunSchema(input map[string]any) (string, error) {
	name, ok := input["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("name is required")
	}
	if t.loader == nil {
		return "", fmt.Errorf("skill loader is not configured")
	}
	skill, err := t.loader.LoadSkill(name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("<skill name=%q>\n%s\n</skill>", skill.Name, skill.Content), nil
}
