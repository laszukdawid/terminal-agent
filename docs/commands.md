# Terminal Agent Commands

Terminal Agent provides several commands to help you interact with the terminal efficiently.

## Available Commands

| Command | Description |
|---------|-------------|
| `ask` | Query the LLM with a question to get information |
| `task` | Execute a task using LLM-generated commands |
| `routine` | Define and run scheduled, unattended agent routines |
| `daemon` | Run and manage the routine scheduler daemon |
| `skill` | List, create, and show reusable task skills |
| `tool` | Manage and execute specific tools |
| `plugin` | Install and manage plugins |
| `config` | Configure Terminal Agent settings |
| `history` | Query your interaction history |
| `memory` | Store ask memories and inspect task memory |

## Task Memory

Task runs are indexed into a local task memory database by default. The index is stored at `~/.local/share/terminal-agent/task_memory.db`; no task memory data is sent anywhere except as part of the normal model prompts needed to answer your current request.

Task memory is used by `agent task` in two ways:

- Before every model iteration, Terminal Agent searches local task memory with a one second budget and adds up to five compact, relevant memories to the task prompt.
- During a task, the agent can use the read-only `memory_search` tool when new information appears, such as an instruction read from a file or an error message that may have been solved before.

Task indexing is asynchronous and best-effort. Task execution does not fail if memory indexing fails or is delayed.

Task memory stores several record types. The most useful records are `tool_success` entries because they include the previous tool name, structured tool input, and output. `task_completion` entries capture the final answer, and `task_request` entries capture that a similar task was asked even when no command has been learned yet.

Search ranks actionable memories first. For example, if a previous task for `what time is it` learned `unix {"command":"date '+%I:%M %p %Z'"}`, that `tool_success` memory is preferred over a bare `task_request` or final-answer-only `task_completion`. This makes the command visible to the model and to session-log validation. Previous outputs in retrieved memories are historical; for current-state requests such as time, date, files, processes, or git state, the agent should reuse the remembered tool input and run it again rather than copying the old output.

The index uses SQLite FTS5 when available. If the local SQLite build does not provide FTS5, Terminal Agent falls back to local lexical scoring over recent memory records. You can see which path is active with `agent memory task stats`.

Useful commands:

```bash
agent memory task search "print current time"
agent memory task stats
agent memory task clear
agent config set task-memory false
agent config set task-memory true
```

The session logs under `~/.local/share/terminal-agent/sessions/` remain the authoritative task history. The SQLite task memory database is a searchable index derived from task activity and can be cleared or rebuilt in future tooling.

To confirm a task used automatic memory retrieval, inspect the newest task session log and look for records with `"status":"task_memory_retrieval"`. These records include the retrieval query, match count, and compact matched memories, including tool inputs for actionable matches. Explicit `memory_search` tool calls also appear as normal tool-call/tool-result records.

Example validation command:

```bash
grep -R '"status":"task_memory_retrieval"' ~/.local/share/terminal-agent/sessions
```

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
- [Routine Command](./commands/routine.md)
- [Daemon Command](./commands/daemon.md)
- [Skills](./skills.md)
- [Tool Command](./commands/tool.md)
- [Plugin Command](./commands/plugin.md)
- [Config Command](./commands/config.md)
- [History Command](./commands/history.md)
