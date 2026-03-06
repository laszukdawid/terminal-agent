package tools

import (
	"maps"

	"github.com/laszukdawid/terminal-agent/internal/config"
)

type toolProvider struct {
	builtinTools map[string]Tool
	mcpTools     map[string]Tool
	allTools     map[string]Tool
	config       config.Config
}

type ToolProvider interface {
	GetAllTools() map[string]Tool
	GetToolByName(name string) Tool
}

func NewToolProvider(config config.Config) ToolProvider {
	allTools := make(map[string]Tool)

	// Initialize the tool provider with all tools
	builtinTools := GetAllBuiltinTools(config)
	maps.Copy(allTools, builtinTools)

	// Load MCP tools if a file path is provided
	mcpFileSchema, _ := LoadMCPFileSchema(config.GetMcpFilePath())
	mcpTools := GetMCPTools(mcpFileSchema)
	maps.Copy(allTools, mcpTools)

	return &toolProvider{
		builtinTools: builtinTools,
		mcpTools:     mcpTools,
		allTools:     allTools,
		config:       config,
	}
}

func (tp *toolProvider) GetAllTools() map[string]Tool {
	return tp.allTools
}

func (tp *toolProvider) GetToolByName(name string) Tool {
	tool, exists := tp.allTools[name]
	if !exists {
		return nil
	}
	return tool
}

func GetAllBuiltinTools(config config.Config) map[string]Tool {
	workDir := ""
	if config != nil {
		workDir = config.GetWorkingDir()
	}

	tools := map[string]Tool{
		NewUnixTool(nil).Name():           NewUnixTool(nil),
		NewFileEditTool(workDir).Name():   NewFileEditTool(workDir),
		NewFileSearchTool(workDir).Name(): NewFileSearchTool(workDir),
		NewPythonTool(workDir).Name():     NewPythonTool(workDir),
	}

	// Add Websearch only if possible
	if websearchTool := NewWebsearchTool(); websearchTool != nil {
		tools[websearchTool.Name()] = websearchTool
	}
	return tools
}

func GetBuiltinToolByName(name string, config config.Config) Tool {
	for _, tool := range GetAllBuiltinTools(config) {
		if tool.Name() == name {
			return tool
		}
	}
	return nil
}
