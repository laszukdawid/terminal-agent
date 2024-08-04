package tools

type Tool interface {
	Run(input *string) (string, error)
	Name() string
	Description() string
}
