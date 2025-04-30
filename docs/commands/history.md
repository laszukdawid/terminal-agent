# History Command

The `history` command allows you to query your past interactions with Terminal Agent.

## Usage

```sh
agent history [flags] [query]
```

The history command will display your previous questions, tasks, and the agent's responses.

## Examples

```sh
# View all history
agent history

# Search history for specific terms
agent history "file descriptor"

# Filter by date range
agent history --after "2023-10-01" --before "2023-10-31"

# Combine filters
agent history --after "2023-10-01" "linux commands"
```

## Flags

| Flag | Description | Format Examples |
|------|-------------|----------------|
| `--after` | Filter logs after this date | `YYYY`, `YYYY-MM-DD`, `HH:MM:SS`, `YYYY-MM-DDTHH:MM:SS` |
| `--before` | Filter logs before this date | `YYYY`, `YYYY-MM-DD`, `HH:MM:SS`, `YYYY-MM-DDTHH:MM:SS` |

## Date Format

When using the `--after` and `--before` flags, the date can be specified in various formats:

- `YYYY` - A specific year (e.g., `2023`)
- `YYYY-MM-DD` - A specific date (e.g., `2023-10-15`)
- `HH:MM:SS` - A specific time today (e.g., `14:30:00`)
- `YYYY-MM-DDTHH:MM:SS` - A specific date and time (e.g., `2023-10-15T14:30:00`)
- `YYYY-MM-DDTHH:MM:SSZ` - A specific date and time with timezone (e.g., `2023-10-15T14:30:00Z`)

When specifying only a time, it's implicitly for today. When specifying only a date, it's implicitly at midnight.

## History Storage

The history logs are stored in a JSONL file at:

```
$HOME/.local/share/terminal-agent/query_log.jsonl
```

## Enabling History Logging

For history to be recorded, you need to use the `--log` or `-l` flag with the `ask` or `task` commands:

```sh
agent ask --log "What is a file descriptor?"
agent task --log "List all files in current directory"
```
