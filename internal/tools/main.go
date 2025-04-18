package tools

func GetAllTools() map[string]Tool {
	tools := map[string]Tool{
		NewUnixTool(nil).Name(): NewUnixTool(nil),
		// NewPythonTool().Name():       NewPythonTool(),
		// NewPythonToolWithContext().Name(): NewPythonToolWithContext(),
	}

	// Add Websearch only if possible
	if websearchTool := NewWebsearchTool(); websearchTool != nil {
		tools[websearchTool.Name()] = websearchTool
	}
	return tools
}

func GetToolByName(name string) Tool {
	for _, tool := range GetAllTools() {
		if tool.Name() == name {
			return tool
		}
	}
	return nil
}
