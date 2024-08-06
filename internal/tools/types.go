package tools

type Tool interface {
	Run(input *string) (string, error)
	Name() string
	Description() string
	DetectTool(input *string) bool
}

type CodeExecutor interface {
	Exec(code string) (string, error)
}
