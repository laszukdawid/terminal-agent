---
layout: default
title: Getting Started with Terminal Agent
---

# Getting Started with Terminal Agent

Terminal Agent is an LLM-powered CLI tool designed to help you interact with your terminal more efficiently.

## Installation

### Download Pre-built Binary (Recommended)

The easiest way to get started is to download the pre-built binary from [GitHub Releases](https://github.com/laszukdawid/terminal-agent/releases):

1. Download the appropriate binary for your system
2. Make it executable: `chmod u+x agent`
3. Move it to your PATH or use it directly: `./agent --help`

### Compile from Source

If you prefer to build from source:

1. Ensure you have Go installed
2. Clone the repository: `git clone https://github.com/laszukdawid/terminal-agent.git`
3. Navigate to the directory: `cd terminal-agent`
4. Build using Taskfile (recommended):
   ```sh
   # Install Taskfile if you don't have it: https://taskfile.dev/installation/
   task build
   ```
5. Install to your path:
   ```sh
   task install
   ```

This will place the `agent` binary in `~/.local/bin/agent`.

## Configuration

Before using Terminal Agent, you'll need to configure it with your preferred LLM provider:

```sh
# Set your provider (e.g., "openai", "anthropic", "bedrock", "perplexity")
agent config set provider openai

# Set your preferred model
agent config set model gpt-4o-mini
```

You'll also need to set the appropriate API key as an environment variable, depending on your provider:

```sh
# For OpenAI
export OPENAI_API_KEY=your_api_key_here

# For Anthropic
export ANTHROPIC_API_KEY=your_api_key_here

# For Perplexity
export PERPLEXITY_KEY=your_api_key_here

# For Amazon Bedrock, configure your AWS credentials as usual
```

## Quick Start

Once installed and configured, you can start using Terminal Agent:

```sh
# Ask a question
agent ask "What is a file descriptor?"

# Execute a task
agent task "List all files in the current directory sorted by size"

# List available tools
agent tool list

# Execute a specific tool
agent tool exec unix "ls -la"
```

## Recommended Aliases

For even quicker access, set up an alias:

```sh
# Add this to your .bashrc or .zshrc
alias aa="agent ask"

# Or run the automatic setup
task install:alias
```

Now you can simply use:

```sh
aa "What is a file descriptor?"
```

## Custom System Prompts

You can customize the system prompt to tailor the agent's behavior for your needs.

### Priority Order

Prompts are resolved in this order (highest to lowest priority):

1. **CLI flag** (`--prompt "your prompt"`)
2. **Project file** in the configured working directory
3. **Default prompt** (built-in)

### File Locations

| Command | File Path (in working directory) |
|---------|----------------------------------|
| `ask`   | `ask/system.prompt` (preferred) or `ask_system.prompt` |
| `task`  | `task/system.prompt` (preferred) or `task_system.prompt` |

### Examples

**Override via CLI flag:**

```sh
agent ask --prompt "You are a helpful coding assistant. Be concise." "How do I reverse a string?"
agent task --prompt "You are a DevOps expert." "Set up a docker compose file"
```

**Using project files:**

```sh
# Set working directory (optional - defaults to config directory)
agent config set working-dir /path/to/your/project

# Create prompt file for ask command
mkdir -p /path/to/your/project/ask
echo "You are a Unix expert. Be concise and provide examples." > /path/to/your/project/ask/system.prompt

# Create prompt file for task command
mkdir -p /path/to/your/project/task
echo "You are an automation expert. Explain each step." > /path/to/your/project/task/system.prompt

# Now use the agent - it will pick up your custom prompts
agent ask "What is a file descriptor?"
agent task "List files sorted by size"
```

### Template Placeholders

All prompts support the `{{header}}` placeholder, which gets replaced with dynamic system context:

```
{{header}}

Your custom instructions here...
```

The header includes: hostname, username, current time, working directory, and OS information.
