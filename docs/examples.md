# Examples

This page provides practical examples of using Terminal Agent for various tasks.

## Ask Command Examples

### Basic Questions

```sh
# Ask about a programming concept
agent ask "What is a closure in JavaScript?"

# Ask about a terminal command
agent ask "How do I find files containing specific text in Linux?"

# Ask about system information
agent ask "What is a process in operating systems?"
```

### Using Different Providers

```sh
# Use OpenAI
agent ask --provider openai "Explain Docker containers vs VMs"

# Use Anthropic
agent ask --provider anthropic "What are the principles of functional programming?"

# Use streaming with a specific model
agent ask --provider bedrock --model "anthropic.claude-3-haiku-20240307-v1:0" --stream "Explain distributed systems"
```

### Formatting Output

```sh
# Get plain text output (no markdown rendering)
agent ask --plain "How do I check free disk space in Linux?"

# Stream the output as it's generated
agent ask --stream "Write a simple Python script to read a CSV file"
```

## Task Command Examples

### File Operations

```sh
# List files
agent task "List all JavaScript files in this directory and its subdirectories"

# Find specific files
agent task "Find all files larger than 100MB"

# File analysis
agent task "Show me the lines of code count for each file type in this project"
```

### System Operations

```sh
# System information
agent task "Show me system resource usage"

# Process management
agent task "List the top 5 processes using the most memory"

# Network analysis
agent task "Check which processes are listening on which ports"
```

### Text Processing

```sh
# Find text in files
agent task "Find all files containing the word 'error' in the logs directory"

# Text extraction
agent task "Extract all email addresses from this text file"

# Text transformation
agent task "Convert this CSV file to a JSON format"
```

## Tool Command Examples

### Unix Tool

```sh
# List files with details
agent tool exec unix "ls -la"

# Find files
agent tool exec unix "find . -name '*.go' -type f | grep -v '_test.go'"

# Process monitoring
agent tool exec unix "ps aux | grep node"

# Using JSON format
agent tool exec unix '{"command": "du -sh * | sort -hr | head -10"}'
```

### Websearch Tool

```sh
# Search for programming information
agent tool exec websearch "golang error handling best practices"

# Search for technical concepts
agent tool exec websearch "microservices vs monolith architecture"

# Search for current information
agent tool exec websearch "latest developments in AI"
```

## Config Command Examples

```sh
# Set up for OpenAI
agent config set provider openai
agent config set model gpt-4o-mini

# Check current configuration
agent config get all

# Set up for Anthropic
agent config set provider anthropic
agent config set model claude-3-5-sonnet-latest

# Configure MCP tools
agent config set mcp-path /home/user/my-tools.json
```

## History Command Examples

```sh
# View all history
agent history

# Search for specific terms in history
agent history "docker"

# Filter by date range
agent history --after "2023-10-01" --before "2023-10-31"

# Combine search and date filters
agent history --after "2023-10-01" "kubernetes"
```

## Advanced Usage Patterns

### Chaining Commands

You can chain Terminal Agent commands with Unix pipes:

```sh
# Get information and save to a file
agent ask "Explain Docker networking" | tee docker-networking.md

# Process the output of a tool
agent tool exec unix "ls -la" | grep "\.go"
```

### Creating Aliases

Set up aliases for frequent operations:

```sh
# Quick ask
alias aa="agent ask"

# Task with specific provider
alias atask="agent task --provider bedrock"

# Quick tool execution
alias unix="agent tool exec unix"
```

### Using with Scripts

You can incorporate Terminal Agent in your scripts:

```bash
#!/bin/bash
# Example script that uses Terminal Agent to analyze logs

LOG_FILE=$1
ERROR_COUNT=$(agent tool exec unix "grep ERROR $LOG_FILE | wc -l")
WARNING_COUNT=$(agent tool exec unix "grep WARNING $LOG_FILE | wc -l")

echo "Found $ERROR_COUNT errors and $WARNING_COUNT warnings"

if [ $ERROR_COUNT -gt 0 ]; then
  agent task "Analyze these log errors and suggest solutions: $(agent tool exec unix "grep ERROR $LOG_FILE | head -10")"
fi
```
