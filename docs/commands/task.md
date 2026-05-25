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

The `task` command runs through an event-driven task execution path. The agent core decides what to do next, while the CLI owns terminal interaction for confirmations and clarification questions.

When the selected provider does not expose dependable native tool calling, Terminal Agent can fall back to a structured JSON action protocol. The direct local `llama` provider uses this path today, so `task` works there as well, although providers with native tool APIs are generally more reliable for complex workflows.

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

## Python Automation (Native)

The task agent can draft and run Python scripts using native tools (no `cat <<EOF`), then execute them with `python3`, `python`, or `uv run python`.

### Prerequisites

- `python3` or `python` available on your PATH
- Optional: `uv` for fast, isolated execution

### Example

```sh
# Write hello.py and run it
agent task "Create hello.py that prints Hello, run it with python3"

# Use uv when available
agent task "Create hello.py that prints Hello, run it with uv run python"
```

If you need PEP 723 script mode, ask for `uv run --script` explicitly.

You will be asked to confirm each execution step, and the agent may also ask follow-up clarification questions when it needs more context. File creation and edits are performed by the agent’s native tools.

## Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--provider` | `-p` | From config | The LLM provider to use |
| `--model` | `-m` | From config | The model ID to use |
| `--print` | `-x` | `true` | Whether to print the response to stdout |
| `--log` | `-l` | `false` | Whether to log the input and output to a file |
| `--plain` | `-k` | `false` | Render the response as plain text (no markdown) |
| `--allow` |  | `[]` | Allow actions without confirmation (repeatable, glob-based) |

Action strings use a function-style format, e.g. `unix("aws login sso")` or `file_edit("README.md", operation="write")`. String values use glob matching against the full value: `*` matches any sequence, `?` matches a single character, and character classes like `[ab]` or `[a-z]` are supported. Escape glob metacharacters with `\` when you want a literal match, for example `unix("ls -d \\*/")`. To constrain keys, use `allowKeys=["region", "profile", "read*"]`, and key values can use the same glob syntax, e.g. `region="us-*"`.

## Safety Features

The task command includes safety measures:

1. **Command Confirmation**: Before executing any generated command, the agent will show you the command and ask for confirmation.
2. **Limited Command Set**: Only certain safe commands are allowed to be executed automatically.
3. **Iterative Approach**: The agent breaks down complex tasks into smaller steps, with visibility at each stage.

## Limitations

Currently, the task command has some limitations:

- Not all Unix commands are supported for automatic execution
- Some advanced operations may require manual intervention
- Direct local `llama` tasks currently use the agent-managed structured fallback rather than provider-native tool calling

## History and Logging

Using the `--log` flag will save your task and the response to the history log:

```sh
agent task --log "Create a backup of all .json files"
```

You can query your history with the `history` command.
