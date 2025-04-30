# Tools

Terminal Agent provides various tools to help you interact with your system and external services.

## Built-in Tools

### unix

The unix tool allows you to execute Unix/bash commands:

```sh
agent tool exec unix "ls -la"
```

Input schema:
```json
{
  "command": "The Unix command to execute (string)"
}
```

This tool provides access to common Unix commands like `ls`, `grep`, `find`, `cat`, etc. For security reasons, not all Unix commands are available.

### websearch

The websearch tool allows you to search the web:

```sh
agent tool exec websearch "golang concurrency patterns"
```

Input schema:
```json
{
  "query": "The search query (string)"
}
```

This tool returns search results with links and snippets.

## Using Tools Directly

You can directly execute tools using the `tool exec` command:

```sh
agent tool exec <tool-name> <input>
```

Examples:
```sh
# List files in current directory
agent tool exec unix "ls -la"

# Search for information
agent tool exec websearch "how to create a git repository"
```

## Using Tools in Tasks

Tools are also used automatically by the `task` command. When you provide a task description, the agent will:

1. Analyze the task
2. Determine which tools are needed
3. Execute the appropriate tools to complete the task

Example:
```sh
agent task "Show me the largest files in the current directory"
```

The agent might use the `unix` tool with commands like `du -sh * | sort -hr` to complete this task.

## Model Context Protocol (MCP) Tools

Terminal Agent supports extending its capabilities through Model Context Protocol (MCP) tools:

1. Create an MCP JSON file defining your tools
2. Configure Terminal Agent to use your MCP file:
   ```sh
   agent config set mcp-path /path/to/mcp.json
   ```
3. Your custom tools will now be available:
   ```sh
   agent tool list
   ```

### MCP File Example

```json
{
  "name": "custom-tools",
  "version": "0.1",
  "tools": [
    {
      "name": "weather",
      "description": "Get weather information for a location",
      "input_schema": {
        "type": "object",
        "properties": {
          "location": {
            "type": "string",
            "description": "The location to get weather for"
          },
          "units": {
            "type": "string",
            "enum": ["celsius", "fahrenheit"],
            "description": "Temperature units"
          }
        },
        "required": ["location"]
      }
    }
  ]
}
```

## Security Considerations

The `unix` tool executes commands on your system, so use caution:

1. Review commands before execution
2. Terminal Agent will ask for confirmation before running commands
3. Some potentially harmful commands are not allowed
4. Use the tool in trusted environments only
