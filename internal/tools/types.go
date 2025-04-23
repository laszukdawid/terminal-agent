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

type CodeExecutor interface {
	Exec(code string) (string, error)
}
