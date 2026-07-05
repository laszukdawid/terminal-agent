package tasktools

import (
	"context"
	"fmt"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/taskmemory"
	"github.com/laszukdawid/terminal-agent/internal/tools"
)

const DefaultLimit = 5

type MemorySearch struct {
	name  string
	store taskmemory.Store
	env   taskmemory.Environment
}

func NewMemorySearch(name string, store taskmemory.Store, env taskmemory.Environment) *MemorySearch {
	return &MemorySearch{name: name, store: store, env: env}
}

func (t *MemorySearch) Name() string { return t.name }

func (t *MemorySearch) PermissionCategory() tools.PermissionCategory { return tools.PermissionRead }

func (t *MemorySearch) Description() string {
	return "Search local task memory for prior tasks, discovered instructions, tool calls, and results relevant to the current task."
}

func (t *MemorySearch) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]string{"type": "string", "description": "The newly discovered subtask, instruction, error, path, command description, or user intent to search for."},
			"limit": map[string]string{"type": "integer", "description": "Optional maximum number of memories to return. Defaults to 5."},
		},
		"required": []string{"query"},
	}
}

func (t *MemorySearch) HelpText() string {
	return "Searches the local task memory index and returns compact prior solutions."
}

func (t *MemorySearch) Run(_ *string) (string, error) {
	return "", fmt.Errorf("memory_search requires structured input")
}

func (t *MemorySearch) RunSchema(input map[string]any) (string, error) {
	query, _ := input["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	limit := DefaultLimit
	if value, ok := input["limit"].(float64); ok && value > 0 {
		limit = int(value)
	} else if value, ok := input["limit"].(int); ok && value > 0 {
		limit = value
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	matches, err := t.store.Search(ctx, taskmemory.Query{Text: query, OS: t.env.OS, Arch: t.env.Arch, Shell: t.env.Shell, CWD: t.env.CWD, Limit: limit, IncludeCWD: true})
	if err != nil {
		return "", err
	}
	return taskmemory.FormatMatches(matches), nil
}
