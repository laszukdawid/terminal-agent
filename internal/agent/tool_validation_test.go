package agent

import (
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateToolInputFileSearchAnyOf(t *testing.T) {
	schema := tools.NewFileSearchTool("").InputSchema()

	err := validateToolInput(schema, map[string]any{})
	require.Error(t, err)
	assert.EqualError(t, err, "input must satisfy at least one allowed field combination")

	assert.NoError(t, validateToolInput(schema, map[string]any{"contains": "task"}))
	assert.NoError(t, validateToolInput(schema, map[string]any{"name_pattern": "*.go"}))
	assert.NoError(t, validateToolInput(schema, map[string]any{"name_pattern": "*.go", "contains": "task"}))
}

func TestValidateToolInputPythonOneOf(t *testing.T) {
	schema := tools.NewPythonTool("").InputSchema()

	err := validateToolInput(schema, map[string]any{})
	require.Error(t, err)
	assert.EqualError(t, err, "input must satisfy exactly one allowed field combination")

	err = validateToolInput(schema, map[string]any{"path": "main.py", "code": "print('hi')"})
	require.Error(t, err)
	assert.EqualError(t, err, "input matches multiple mutually exclusive field combinations")

	assert.NoError(t, validateToolInput(schema, map[string]any{"path": "main.py"}))
	assert.NoError(t, validateToolInput(schema, map[string]any{"code": "print('hi')"}))
}
