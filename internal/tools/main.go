package tools

func GetAllTools() map[string]Tool {
	tools := map[string]Tool{
		NewUnixTool(nil).Name():   NewUnixTool(nil),
		NewWebsearchTool().Name(): NewWebsearchTool(),
		// NewPythonTool().Name():       NewPythonTool(),
		// NewPythonToolWithContext().Name(): NewPythonToolWithContext(),
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
