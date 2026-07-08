package connector

import (
	"encoding/json"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequiredFields(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
		want  []string
	}{
		{name: "native []string from built-in tools", input: map[string]any{"required": []string{"command"}}, want: []string{"command"}},
		{name: "json-decoded []any from MCP tools", input: map[string]any{"required": []any{"a", "b"}}, want: []string{"a", "b"}},
		{name: "[]any drops non-string elements", input: map[string]any{"required": []any{"a", 42, nil, "b"}}, want: []string{"a", "b"}},
		{name: "empty []any", input: map[string]any{"required": []any{}}, want: []string{}},
		{name: "missing key", input: map[string]any{}, want: nil},
		{name: "wrong type", input: map[string]any{"required": "command"}, want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, requiredFields(tc.input))
		})
	}
}

// mcpStyleTool mimics an MCP tool: its InputSchema is produced by a JSON
// round-trip, so "required" decodes to []any rather than the []string that
// built-in tools construct natively. Built-in tools cannot reproduce the bug
// this guards against, which is why the tests use this fake instead.
type mcpStyleTool struct {
	name   string
	schema map[string]any
}

func (m mcpStyleTool) Name() string                               { return m.name }
func (m mcpStyleTool) Description() string                        { return "mcp style tool" }
func (m mcpStyleTool) InputSchema() map[string]any                { return m.schema }
func (m mcpStyleTool) HelpText() string                           { return "" }
func (m mcpStyleTool) Run(_ *string) (string, error)              { return "", nil }
func (m mcpStyleTool) RunSchema(_ map[string]any) (string, error) { return "", nil }

func newMCPStyleTool(t *testing.T, name string) mcpStyleTool {
	t.Helper()
	const raw = `{"type":"object","properties":{"command":{"type":"string","description":"The command to run."}},"required":["command"]}`
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &schema))
	// Guard the fixture itself: if the round-trip ever yielded []string this
	// test would silently stop reproducing the MCP condition.
	_, isAnySlice := schema["required"].([]any)
	require.True(t, isAnySlice, "MCP-style schema must decode required as []any")
	return mcpStyleTool{name: name, schema: schema}
}

// TestConvertToolsToAnthropicPreservesMCPRequired guards BUG-1 for Anthropic:
// an MCP tool's required list must survive schema conversion.
func TestConvertToolsToAnthropicPreservesMCPRequired(t *testing.T) {
	tool := newMCPStyleTool(t, "mcp_tool")

	out := convertToolsToAnthropic(map[string]tools.Tool{tool.Name(): tool})
	require.Len(t, out, 1)

	required, ok := out[0].InputSchema.ExtraFields["required"].([]string)
	require.True(t, ok, "required must be carried through to the Anthropic schema")
	assert.Equal(t, []string{"command"}, required)
}

// TestConvertToolsToGooglePreservesMCPRequired guards BUG-1 for Google.
func TestConvertToolsToGooglePreservesMCPRequired(t *testing.T) {
	tool := newMCPStyleTool(t, "mcp_tool")

	schema, err := convertInputSchemaToGenaiSchema(tool.InputSchema())
	require.NoError(t, err)
	assert.Contains(t, schema.Required, "command")
}
