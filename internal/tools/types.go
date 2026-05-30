package tools

import "context"

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

type ToolExecutionContext struct {
	RootDir    string
	CurrentDir string
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

type LlmResponseWithTools struct {
	ToolName     string
	ToolInput    map[string]any
	ToolResponse string

	Response string
}
