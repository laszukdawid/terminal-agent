package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/utils"
	mcpClient "github.com/mark3labs/mcp-go/client"
	mcpMain "github.com/mark3labs/mcp-go/mcp"
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
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// MCPTool is a wrapper that adapts an MCP server to the Tool interface
type MCPTool struct {
	name        string
	description string
	// server      MCPServer
	inputSchema mcpMain.ToolInputSchema
	client      *mcpClient.Client
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
	inputSchema, err := utils.ConvertMCPSchemaToMap(t.inputSchema)
	if err != nil {
		fmt.Printf("Error converting MCP schema to map: %v\n", err)
		return nil
	}
	return inputSchema
}

// HelpText returns the help text for the MCP tool
func (t *MCPTool) HelpText() string {
	return fmt.Sprintf("Help for %s: %s\n\n%s",
		t.Name(), t.Description(), t.InputSchema())
}

func (t *MCPTool) RunSchema(input map[string]any) (string, error) {

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// // Remove the "cmd" key from the input map
	fmt.Printf("Command: %s\n", t.Name())
	fmt.Printf("Arguments: %v\n", input)

	request := mcpMain.CallToolRequest{}
	request.Params.Name = t.Name()
	request.Params.Arguments = input
	result, err := t.client.CallTool(ctx, request)
	if err != nil {
		return "", fmt.Errorf("error calling tool %s: %w", t.Name(), err)
	}

	// Build response from the result
	parsedResponse := ""
	for _, content := range result.Content {
		if textContent, ok := content.(mcpMain.TextContent); ok {
			parsedResponse += textContent.Text
		} else {
			jsonBytes, _ := json.Marshal(content)
			parsedResponse += string(jsonBytes)
		}
	}

	return parsedResponse, nil
}

// RunSchema processes a structured input for the MCP tool
func (t *MCPTool) Run(input *string) (string, error) {
	// Convert input to JSON
	inputMap := map[string]any{
		"cmd": *input,
	}

	return t.RunSchema(inputMap)
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
		serverTools, err := getServerAllTools(server)
		if err != nil {
			fmt.Printf("couldn't list tools for server %s. Skipping.", name)
		}
		maps.Copy(tools, serverTools)
	}

	return tools
}

// getServerAllTools creates a map of tools for a given server
func getServerAllTools(server MCPServer) (map[string]Tool, error) {
	tools := make(map[string]Tool)

	flatEnv := make([]string, 0, len(server.Env))
	for key, value := range server.Env {
		flatEnv = append(flatEnv, fmt.Sprintf("%s=%s", key, value))
	}

	// Pool server for all tools
	c, err := mcpClient.NewStdioMCPClient(
		server.Command,
		flatEnv,
		server.Args...,
	)
	if err != nil {
		return nil, fmt.Errorf("error creating MCP client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	initRequest := mcpMain.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcpMain.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcpMain.Implementation{
		Name:    server.Command,
		Version: "0.1",
	}

	initResult, err := c.Initialize(ctx, initRequest)
	if err != nil {
		return nil, fmt.Errorf("error initializing MCP client: %w", err)
	}

	fmt.Printf(
		"Initialized with server: %s %s\n\n",
		initResult.ServerInfo.Name,
		initResult.ServerInfo.Version,
	)

	// List all tools
	toolsRequest := mcpMain.ListToolsRequest{}
	mcpTools, err := c.ListTools(ctx, toolsRequest)
	if err != nil {
		return nil, fmt.Errorf("error listing tools: %w", err)
	}

	for _, tool := range mcpTools.Tools {
		tools[tool.Name] = &MCPTool{
			name:        tool.Name,
			description: tool.Description,
			inputSchema: tool.InputSchema,
			client:      c,
		}
	}

	return tools, nil
}
