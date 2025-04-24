package utils

import (
	"encoding/json"

	mcpMain "github.com/mark3labs/mcp-go/mcp"
)

// ConvertMCPSchemaToMap converts an mcpMain.ToolInputSchema to a map[string]any
// that can be used by the Tool interface
func ConvertMCPSchemaToMap(schema mcpMain.ToolInputSchema) (map[string]any, error) {
	// First convert to JSON bytes
	jsonBytes, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}

	// Then unmarshal into map[string]any
	var result map[string]any
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, err
	}

	return result, nil
}
