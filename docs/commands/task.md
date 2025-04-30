# Task Command

The `task` command enables you to describe a task you want to accomplish, and the agent will generate and execute commands to complete it.

## Usage

```sh
agent task [flags] <task description>
```

The agent will:
1. Interpret your task description
2. Generate appropriate commands to accomplish the task
3. Ask for your permission before executing any commands
4. Provide the results

## Examples

```sh
# List files
agent task "List all files in the current directory sorted by size"

# Find specific files
agent task "Find all .go files modified in the last week"

# Process text
agent task "Count the number of lines in all .md files"

# System information
agent task "Show me information about my CPU and memory usage"
```

## Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--provider` | `-p` | From config | The LLM provider to use |
| `--model` | `-m` | From config | The model ID to use |
| `--print` | `-x` | `true` | Whether to print the response to stdout |
| `--log` | `-l` | `false` | Whether to log the input and output to a file |
| `--plain` | `-k` | `false` | Render the response as plain text (no markdown) |

## Safety Features

The task command includes safety measures:

1. **Command Confirmation**: Before executing any generated command, the agent will show you the command and ask for confirmation.
2. **Limited Command Set**: Only certain safe commands are allowed to be executed automatically.
3. **Iterative Approach**: The agent breaks down complex tasks into smaller steps, with visibility at each stage.

## Limitations

Currently, the task command has some limitations:

- Not all Unix commands are supported for automatic execution
- Some advanced operations may require manual intervention
- The task command is not supported by the Perplexity provider

## History and Logging

Using the `--log` flag will save your task and the response to the history log:

```sh
agent task --log "Create a backup of all .json files"
```

You can query your history with the `history` command.
