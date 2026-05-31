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

When task tools run long-lived foreground commands, their stdout/stderr can be streamed to the terminal while the command is still running. This live stream is controlled by the same `--print` flag as the final response: with the default `--print=true`, output is printed as soon as it is available; with `--print=false`, live tool output and the final response are both suppressed while the task still consumes the event stream internally.

If Terminal Agent cannot display live output but the process is still running, it emits a warning event that includes the process id when available. If the display event path itself is unavailable, the warning is written to logs instead.

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
| `--timeout` |  | unlimited | Maximum duration for the whole task run (Go duration, e.g. `90s`, `15m`, `2h`); `0` means no timeout |

Action strings use a function-style format, e.g. `unix("aws login sso")` or `file_edit("README.md", operation="write")`. String values use glob matching against the full value: `*` matches any sequence, `?` matches a single character, and character classes like `[ab]` or `[a-z]` are supported. Escape glob metacharacters with `\` when you want a literal match, for example `unix("ls -d \\*/")`. To constrain keys, use `allowKeys=["region", "profile", "read*"]`, and key values can use the same glob syntax, e.g. `region="us-*"`.

## Task Timeout

By default a task runs with **no timeout** — useful for long-running, foreground work such as monitoring (`agent task "watch CPU usage and tell me when it crosses 50%"`). The run stops only when the task completes, you cancel it (Ctrl-C), or it hits the internal max tool-call/turn limits.

Use `--timeout` to bound the whole run with a Go duration string:

```sh
# Stop the task after 15 minutes
agent task --timeout 15m "tail the log and summarize errors as they appear"

# Short bound for a quick job
agent task --timeout 90s "run the test suite and report failures"
```

Resolution order, highest priority first:

1. The `--timeout` flag (including an explicit `--timeout 0`, which forces unlimited even when a config default is set).
2. The `task_timeout` value in `~/.config/terminal-agent/config.json` (a duration string such as `"15m"`).
3. The built-in default: unlimited.

```json
{
  "task_timeout": "15m"
}
```

When a task stops because its timeout elapsed, the run fails with a distinct timeout error so it is distinguishable from tool or model failures. The resolved timeout (`"15m"` or `"unlimited"`) is recorded in the session log header so you can audit why a task stopped. Cancelling the caller's context (e.g. pressing Ctrl-C) always takes precedence over the timeout.

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
