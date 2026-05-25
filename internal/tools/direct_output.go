package tools

const DirectOutputSchemaFlag = "x-terminal-agent-direct-output"

func NewFinalOutputField(description string) map[string]any {
	return map[string]any{
		"type":                 "boolean",
		"description":          description,
		DirectOutputSchemaFlag: true,
	}
}
