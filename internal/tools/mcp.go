package tools

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"strings"
)

// MCPFileSchema represents a Model Context Protocol definition file structure
type MCPFileSchema struct {
	Inputs  []MCPInput           `json:"inputs"`
	Servers map[string]MCPServer `json:"servers"`
}

type MCPInput struct {
	Type        string `json:"type"`
	Id          string `json:"id,omitempty"`
	Description string `json:"description,omitempty"`
	Password    bool   `json:"password,omitempty"`
}

// MCPServer represents a server configuration
type MCPServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// MCPTool is a wrapper that adapts an MCP server to the Tool interface
type MCPTool struct {
	name        string
	description string
	server      MCPServer
}

// Name returns the name of the MCP tool
func (t *MCPTool) Name() string {
	return t.name
}

// Description returns the description of the MCP tool
func (t *MCPTool) Description() string {
	return t.description
}

// InputSchema returns the input schema of the MCP tool
func (t *MCPTool) InputSchema() map[string]any {
	return map[string]any{
		"type":        "string",
		"description": "Input for the MCP tool",
	}
}

// HelpText returns the help text for the MCP tool
func (t *MCPTool) HelpText() string {
	return fmt.Sprintf("Help for %s: %s\n\nServer: %s %s",
		t.Name(), t.Description(), t.server.Command, strings.Join(t.server.Args, " "))
}

// Run processes a string input for the MCP tool
func (t *MCPTool) Run(input *string) (string, error) {
	if input == nil {
		return "", fmt.Errorf("input is nil")
	}

	// Prepare the command
	cmd := exec.Command(t.server.Command, t.server.Args...)

	// Set environment variables
	if t.server.Env != nil {
		cmd.Env = os.Environ()
		for key, value := range t.server.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}
	}

	// Prepare the input
	cmd.Stdin = strings.NewReader(*input)

	// Execute the command
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error executing MCP server: %w\nOutput: %s", err, string(output))
	}

	return string(output), nil
}

// RunSchema processes a structured input for the MCP tool
func (t *MCPTool) RunSchema(input map[string]any) (string, error) {
	// Convert input to JSON
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("error encoding input to JSON: %w", err)
	}

	inputStr := string(inputJSON)
	return t.Run(&inputStr)
}

// LoadMCPFileSchema loads an MCP configuration from a JSON file
func LoadMCPFileSchema(filePath string) (*MCPFileSchema, error) {
	if filePath == "" {
		return nil, fmt.Errorf("no MCP file path provided")
	}

	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read MCP file: %w", err)
	}

	// Parse the JSON
	var mcpConfig MCPFileSchema
	if err := json.Unmarshal(data, &mcpConfig); err != nil {
		return nil, fmt.Errorf("failed to parse MCP JSON: %w", err)
	}

	return &mcpConfig, nil
}

// GetMCPTools converts an MCP configuration to a map of tools
func GetMCPTools(mcpConfig *MCPFileSchema) map[string]Tool {
	if mcpConfig == nil || len(mcpConfig.Servers) == 0 {
		return map[string]Tool{}
	}

	tools := make(map[string]Tool)
	for name, server := range mcpConfig.Servers {
		// Create a tool for each server
		tool := &MCPTool{
			name:        name,
			description: fmt.Sprintf("MCP server tool: %s", name),
			server:      server,
		}
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
	mcpConfig, err := LoadMCPFileSchema(mcpFilePath)
	if err != nil {
		fmt.Printf("Warning: Failed to load MCP file: %v\n", err)
		return tools
	}

	// Add the MCP tools
	mcpTools := GetMCPTools(mcpConfig)
	maps.Copy(tools, mcpTools)

	return tools
}
