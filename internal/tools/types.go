package tools

import (
	"context"
	"io"
)

type Tool interface {

	// Runs the tool on the input. Execpts the input to be in accordance with the input schema.
	RunSchema(input map[string]any) (string, error)

	// Shortcut for RunSchema in case the input has a single string value.
	Run(input *string) (string, error)

	Name() string
	Description() string
	InputSchema() map[string]any

	// HelpText returns detailed help information for the tool
	HelpText() string
}

type TaskSchemaProvider interface {
	TaskInputSchema() map[string]any
}

type StatusFormatter interface {
	ToolStatus(input map[string]any) string
}

func EffectiveTaskInputSchema(tool Tool) map[string]any {
	if provider, ok := tool.(TaskSchemaProvider); ok {
		return provider.TaskInputSchema()
	}
	return tool.InputSchema()
}

type AvailabilityAwareTool interface {
	Tool
	IsAvailable() bool
}

// PermissionCategory describes a tool's blast radius for confirmation policy.
type PermissionCategory string

const (
	PermissionRead    PermissionCategory = "read"
	PermissionWrite   PermissionCategory = "write"
	PermissionExecute PermissionCategory = "execute"
)

// CategorizedTool lets a tool declare its permission category. Tools that do
// not implement it are treated as PermissionExecute (gated).
type CategorizedTool interface {
	Tool
	PermissionCategory() PermissionCategory
}

// ProcessesStartedWriter is an optional live-output sink extension for tools that
// launch local processes. It lets callers correlate streamed output or display
// warnings with the OS process that is still running.
type ProcessesStartedWriter interface {
	io.Writer
	ProcessStarted(pid int)
}

type ToolExecutionContext struct {
	RootDir    string
	CurrentDir string
	// Output is an optional live, user-facing output sink. Tools should still
	// return their captured result for agent reasoning; Output is only for
	// incremental display and may be ignored by tools without useful live output.
	Output io.Writer
	// Progress is an optional semantic progress sink. It is separate from Output:
	// progress is for user-facing status updates, not captured command output.
	Progress func(string)
}

type ContextualTool interface {
	Tool
	RunSchemaWithContext(input map[string]any, ctx ToolExecutionContext) (string, error)
}

type ContextAwareTool interface {
	Tool
	RunSchemaContext(ctx context.Context, input map[string]any, execCtx ToolExecutionContext) (string, error)
}

type CodeExecutor interface {
	Exec(code string) (string, error)
}

type ContextAwareCodeExecutor interface {
	CodeExecutor
	ExecContext(ctx context.Context, code string) (string, error)
}

type ProcessOptionsCodeExecutor interface {
	ContextAwareCodeExecutor
	ExecContextWithOptions(ctx context.Context, code string, opts ProcessOptions) (ProcessResult, error)
}

type LlmResponseWithTools struct {
	ToolName     string
	ToolInput    map[string]any
	ToolResponse string

	Response string
}
