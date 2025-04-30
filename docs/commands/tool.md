# Tool Command

The `tool` command allows you to manage and directly execute specific tools available in Terminal Agent.

## Usage

```sh
agent tool <subcommand> [flags] [args]
```

## Subcommands

### list

Lists all available tools:

```sh
agent tool list
```

### help

Shows detailed help information for a specific tool:

```sh
agent tool help <toolName>
```

### exec

Executes a specific tool directly:

```sh
agent tool exec <toolName> <input>
```

## Examples

```sh
# List all available tools
agent tool list

# Get help for the unix tool
agent tool help unix

# Execute a unix command
agent tool exec unix "ls -la"

# Execute a command with complex input (JSON format)
agent tool exec unix '{"command": "find . -name \"*.go\" -type f | wc -l"}'
```

## Available Tools

Terminal Agent comes with several built-in tools:

### unix

Execute Unix/bash commands:

```sh
agent tool exec unix "ls -la"
```

### websearch

Search the web for information:

```sh
agent tool exec websearch "go language benefits"
```

### MCP Tools

If you've configured Model Context Protocol (MCP) tools, they'll also be available through the tool command:

```sh
# First, set up your MCP file
agent config set mcp-path /path/to/mcp.json

# Then list tools to see new MCP-based tools
agent tool list

# Execute an MCP tool
agent tool exec <mcp-tool-name> <input>
```

## Input Format

Tools accept input in different formats:

1. **Plain string**: Simple string input for basic commands
   ```sh
   agent tool exec unix "ls -la"
   ```

2. **JSON string**: For more complex inputs with specific parameters
   ```sh
   agent tool exec unix '{"command": "find . -name \"*.md\" -type f"}'
   ```

## Safety Considerations

When using the unix tool or other tools that execute commands, be careful with commands that could modify your system. Always review commands before execution.
