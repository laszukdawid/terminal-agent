package tools

import (
	"encoding/json"
	"fmt"
	"os"
)

// MCPDefinition represents a Model Context Protocol definition file structure
type MCPDefinition struct {
	Schema     string        `json:"schema"`
	Name       string        `json:"name"`
	Functions  []MCPFunction `json:"functions"`
	Properties MCPProperties `json:"properties"`
}

// MCPFunction represents a function in the MCP definition
type MCPFunction struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Parameters  MCPParameters `json:"parameters"`
}

// MCPParameters represents the parameters of an MCP function
type MCPParameters struct {
	Type       string                 `json:"type"`
	Properties map[string]MCPProperty `json:"properties"`
	Required   []string               `json:"required"`
}

// MCPProperty represents a property in the MCP parameters
type MCPProperty struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Enum        []string    `json:"enum,omitempty"`
	Items       *MCPItem    `json:"items,omitempty"`
	Default     interface{} `json:"default,omitempty"`
}

// MCPItem represents an item in a property array
type MCPItem struct {
	Type string `json:"type"`
}

// MCPProperties represents the top-level properties in an MCP definition
type MCPProperties struct {
	MaxTokens MCPProperty `json:"max_tokens"`
}

// MCPTool is a wrapper that adapts an MCP function to the Tool interface
type MCPTool struct {
	function MCPFunction
}

// Name returns the name of the MCP tool
func (t *MCPTool) Name() string {
	return t.function.Name
}

// Description returns the description of the MCP tool
func (t *MCPTool) Description() string {
	return t.function.Description
}

// InputSchema returns the input schema of the MCP tool
func (t *MCPTool) InputSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": t.function.Parameters.Properties,
		"required":   t.function.Parameters.Required,
	}
}

// HelpText returns the help text for the MCP tool
func (t *MCPTool) HelpText() string {
	return fmt.Sprintf("Help for %s: %s\n%s", t.Name(), t.Description(), t.function.Parameters)
}

// Run processes a string input for the MCP tool
func (t *MCPTool) Run(input *string) (string, error) {
	if input == nil {
		return "", fmt.Errorf("input is nil")
	}
	return fmt.Sprintf("MCP tool %s invoked with: %s", t.Name(), *input), nil
}

// RunSchema processes a structured input for the MCP tool
func (t *MCPTool) RunSchema(input map[string]any) (string, error) {
	return fmt.Sprintf("MCP tool %s executed with structured input", t.Name()), nil
}

// LoadMCPFromFile loads an MCP definition from a JSON file
func LoadMCPFromFile(filePath string) (*MCPDefinition, error) {
	if filePath == "" {
		return nil, fmt.Errorf("no MCP file path provided")
	}

	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read MCP file: %w", err)
	}

	// Parse the JSON
	var mcp MCPDefinition
	if err := json.Unmarshal(data, &mcp); err != nil {
		return nil, fmt.Errorf("failed to parse MCP JSON: %w", err)
	}

	return &mcp, nil
}

// GetMCPTools converts an MCP definition to a map of tools
func GetMCPTools(mcp *MCPDefinition) map[string]Tool {
	if mcp == nil || len(mcp.Functions) == 0 {
		return map[string]Tool{}
	}

	tools := make(map[string]Tool)
	for _, function := range mcp.Functions {
		tool := &MCPTool{function: function}
		tools[tool.Name()] = tool
	}

	return tools
}

// GetAllToolsWithMCP returns all tools including those loaded from the MCP file
func GetAllToolsWithMCP(mcpFilePath string) map[string]Tool {
	// Start with the built-in tools
	tools := GetAllTools()

	// If no MCP file path is provided, just return the built-in tools
	if mcpFilePath == "" {
		return tools
	}

	// Try to load the MCP file
	mcp, err := LoadMCPFromFile(mcpFilePath)
	if err != nil {
		fmt.Printf("Warning: Failed to load MCP file: %v\n", err)
		return tools
	}

	// Add the MCP tools
	mcpTools := GetMCPTools(mcp)
	for name, tool := range mcpTools {
		tools[name] = tool
	}

	return tools
}
