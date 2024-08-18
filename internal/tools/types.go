package tools

type Tool interface {

	// Runs the tool on the input. Execpts the input to be in accordance with the input schema.
	RunSchema(input map[string]interface{}) (string, error)

	// Shortcut for RunSchema in case the input has a single string value.
	Run(input *string) (string, error)

	Name() string
	Description() string
	InputSchema() map[string]interface{}

	// Detects if the input is for this tool. This is used to determine if the tool should be run.
	DetectTool(input *string) (string, error)
}

type CodeExecutor interface {
	Exec(code string) (string, error)
}
