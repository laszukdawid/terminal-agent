package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToolStatusFormatting(t *testing.T) {
	tests := []struct {
		name  string
		tool  StatusFormatter
		input map[string]any
		want  string
	}{
		{
			name:  "read path",
			tool:  NewReadTool(""),
			input: map[string]any{"path": "internal/agent/task.go"},
			want:  `Read(internal/agent/task.go)`,
		},
		{
			name:  "read path with pagination",
			tool:  NewReadTool(""),
			input: map[string]any{"path": "internal/agent/task.go", "offset": 10, "limit": 20},
			want:  `Read(internal/agent/task.go) offset=10 limit=20`,
		},
		{
			name:  "file search pattern",
			tool:  NewFileSearchTool(""),
			input: map[string]any{"name_pattern": "*.go"},
			want:  `Search(*.go)`,
		},
		{
			name:  "file search contains and root",
			tool:  NewFileSearchTool(""),
			input: map[string]any{"contains": "TaskState", "root": "internal/agent"},
			want:  `Search(internal/agent) TaskState`,
		},
		{
			name:  "file search pattern and contains",
			tool:  NewFileSearchTool(""),
			input: map[string]any{"name_pattern": "*.go", "contains": "TaskState"},
			want:  `Search(*.go) TaskState`,
		},
		{
			name:  "file search root pattern and contains",
			tool:  NewFileSearchTool(""),
			input: map[string]any{"root": "internal/agent", "name_pattern": "*.go", "contains": "TaskState"},
			want:  `Search(internal/agent/*.go) TaskState`,
		},
		{
			name:  "file search contains only",
			tool:  NewFileSearchTool(""),
			input: map[string]any{"contains": "TaskState"},
			want:  `Search() TaskState`,
		},
		{
			name:  "file edit write",
			tool:  NewFileEditTool(""),
			input: map[string]any{"operation": "write", "path": "README.md"},
			want:  `Edit(README.md) write`,
		},
		{
			name:  "file edit replace",
			tool:  NewFileEditTool(""),
			input: map[string]any{"operation": "replace", "path": "internal/agent/task.go"},
			want:  `Edit(internal/agent/task.go) replace`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.tool.ToolStatus(tt.input))
		})
	}
}
