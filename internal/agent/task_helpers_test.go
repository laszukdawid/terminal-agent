package agent

import (
	"bytes"
	"context"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contextAwareTaskTool struct {
	receivedCtx   context.Context
	receivedInput map[string]any
	receivedExec  tools.ToolExecutionContext
	legacyCalled  bool
}

func (t *contextAwareTaskTool) Name() string                { return "context_aware" }
func (t *contextAwareTaskTool) Description() string         { return "" }
func (t *contextAwareTaskTool) InputSchema() map[string]any { return map[string]any{} }
func (t *contextAwareTaskTool) HelpText() string            { return "" }
func (t *contextAwareTaskTool) RunSchema(map[string]any) (string, error) {
	t.legacyCalled = true
	return "legacy", nil
}
func (t *contextAwareTaskTool) Run(*string) (string, error) { return "legacy", nil }
func (t *contextAwareTaskTool) RunSchemaContext(ctx context.Context, input map[string]any, execCtx tools.ToolExecutionContext) (string, error) {
	t.receivedCtx = ctx
	t.receivedInput = input
	t.receivedExec = execCtx
	return "context-aware", nil
}

type legacyTaskTool struct {
	receivedInput map[string]any
}

func (t *legacyTaskTool) Name() string                { return "legacy" }
func (t *legacyTaskTool) Description() string         { return "" }
func (t *legacyTaskTool) InputSchema() map[string]any { return map[string]any{} }
func (t *legacyTaskTool) HelpText() string            { return "" }
func (t *legacyTaskTool) RunSchema(input map[string]any) (string, error) {
	t.receivedInput = input
	return "legacy", nil
}
func (t *legacyTaskTool) Run(*string) (string, error) { return "legacy", nil }

func TestRunTaskTool(t *testing.T) {
	t.Run("passes task context to context-aware tool", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		input := map[string]any{"command": "pwd"}
		tool := &contextAwareTaskTool{}
		dirs := TaskDirs{RootDir: "/repo", CurrentDir: "/repo/internal"}

		output, err := runTaskTool(ctx, tool, input, dirs, nil)

		require.NoError(t, err)
		assert.Equal(t, "context-aware", output)
		assert.False(t, tool.legacyCalled)
		assert.Equal(t, input, tool.receivedInput)
		assert.Equal(t, dirs.RootDir, tool.receivedExec.RootDir)
		assert.Equal(t, dirs.CurrentDir, tool.receivedExec.CurrentDir)
		assert.Nil(t, tool.receivedExec.Output)
		assert.Equal(t, ctx, tool.receivedCtx)
		cancel()
		require.ErrorIs(t, tool.receivedCtx.Err(), context.Canceled)
	})

	t.Run("passes output writer to context-aware tool", func(t *testing.T) {
		input := map[string]any{"command": "pwd"}
		tool := &contextAwareTaskTool{}
		dirs := TaskDirs{RootDir: "/repo", CurrentDir: "/repo/internal"}
		var liveOutput bytes.Buffer

		output, err := runTaskTool(context.Background(), tool, input, dirs, &liveOutput)

		require.NoError(t, err)
		assert.Equal(t, "context-aware", output)
		assert.Same(t, &liveOutput, tool.receivedExec.Output)
	})

	t.Run("falls back to legacy tool", func(t *testing.T) {
		input := map[string]any{"value": "ok"}
		tool := &legacyTaskTool{}

		output, err := runTaskTool(context.Background(), tool, input, TaskDirs{}, nil)

		require.NoError(t, err)
		assert.Equal(t, "legacy", output)
		assert.Equal(t, input, tool.receivedInput)
	})
}
