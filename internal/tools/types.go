package tools

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

type ToolExecutionContext struct {
	RootDir    string
	CurrentDir string
}

type ContextualTool interface {
	Tool
	RunSchemaWithContext(input map[string]any, ctx ToolExecutionContext) (string, error)
}

type CodeExecutor interface {
	Exec(code string) (string, error)
}

type LlmResponseWithTools struct {
	ToolName     string
	ToolInput    map[string]any
	ToolResponse string

	Response string
}
