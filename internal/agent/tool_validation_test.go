package agent

import (
	"encoding/json"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateToolInputFileSearchAnyOf(t *testing.T) {
	schema := tools.EffectiveTaskInputSchema(tools.NewFileSearchTool(""))

	err := validateToolInput(schema, map[string]any{})
	require.Error(t, err)
	assert.EqualError(t, err, "input must satisfy at least one allowed field combination")

	assert.NoError(t, validateToolInput(schema, map[string]any{"contains": "task"}))
	assert.NoError(t, validateToolInput(schema, map[string]any{"name_pattern": "*.go"}))
	assert.NoError(t, validateToolInput(schema, map[string]any{"name_pattern": "*.go", "contains": "task"}))
}

func TestValidateToolInputPythonOneOf(t *testing.T) {
	schema := tools.EffectiveTaskInputSchema(tools.NewPythonTool(""))

	err := validateToolInput(schema, map[string]any{})
	require.Error(t, err)
	assert.EqualError(t, err, "input must satisfy exactly one allowed field combination")

	err = validateToolInput(schema, map[string]any{"path": "main.py", "code": "print('hi')"})
	require.Error(t, err)
	assert.EqualError(t, err, "input matches multiple mutually exclusive field combinations")

	assert.NoError(t, validateToolInput(schema, map[string]any{"path": "main.py"}))
	assert.NoError(t, validateToolInput(schema, map[string]any{"code": "print('hi')"}))
}

func TestValidateToolInputNormalizesIntegerFields(t *testing.T) {
	schema := tools.EffectiveTaskInputSchema(tools.NewFileSearchTool(""))
	input := map[string]any{
		"contains":    "task",
		"max_results": json.Number("5"),
	}

	require.NoError(t, validateToolInput(schema, input))
	normalizeToolInput(schema, input)
	assert.Equal(t, 5, input["max_results"])
}
