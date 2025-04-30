# Terminal Agent Commands

Terminal Agent provides several commands to help you interact with the terminal efficiently.

## Available Commands

| Command | Description |
|---------|-------------|
| `ask` | Query the LLM with a question to get information |
| `task` | Execute a task using LLM-generated commands |
| `tool` | Manage and execute specific tools |
| `config` | Configure Terminal Agent settings |
| `history` | Query your interaction history |

## Common Flags

Most commands support these flags:

| Flag | Short | Description |
|------|-------|-------------|
| `--provider` | `-p` | The LLM provider to use (e.g., "openai", "anthropic") |
| `--model` | `-m` | The specific model ID to use |
| `--loglevel` | | Set logging level (debug, info, warn, error, etc.) |
| `--print` | `-x` | Whether to print the response to stdout |
| `--log` | `-l` | Whether to log interactions to history |
| `--plain` | `-k` | Display output as plain text (no markdown formatting) |

## Command Details

For detailed information about each command, see:

- [Ask Command](./commands/ask.md)
- [Task Command](./commands/task.md)
- [Tool Command](./commands/tool.md)
- [Config Command](./commands/config.md)
- [History Command](./commands/history.md)
