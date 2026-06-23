# Routine Command

The `routine` command group defines and runs **routines**: saved, unattended agent runs. A
routine bundles a prompt, an optional schedule, resource budgets (time, tokens, steps), a
model, and a tool/permission posture, then runs the same task pipeline as `agent task` but
without a human present.

Routine definitions live in `~/.config/terminal-agent/routines.json`. Run results live in a
dedicated directory, `~/.local/share/terminal-agent/routines/`, separate from the shared
session logs.

## Usage

```sh
agent routine <subcommand> [flags]
```

| Subcommand | Purpose |
|------------|---------|
| `create`   | Create a routine |
| `list`     | List routines with status, last/next run, schedule, model, and prompt preview |
| `show <id>`| Show a routine's full definition and last run |
| `run <id>` | Run a routine now |
| `enable <id>` / `disable <id>` | Toggle whether a routine is active |
| `delete <id>` | Delete a routine (use `--purge` to also remove its stored runs) |
| `logs <id>` | List a routine's run logs, or `--last` to print the latest summary |

Routines are referenced by id, by exact name, or by an unambiguous id prefix.

## Creating a routine

```sh
agent routine create \
  --name "Daily standup" \
  --cron "0 9 * * 1-5" \
  --model gpt-4o-mini \
  --prompt "Summarize my git commits from yesterday"
```

When you create a routine **with a schedule** and the [scheduler daemon](daemon.md) is
not yet running, `create` offers to install and start it (in an interactive terminal),
so scheduled routines actually fire. Decline it and you can set it up later with
`agent daemon install`.

The prompt may be supplied with `--prompt`, `--prompt-file <path>`, or as positional
arguments. Key flags:

| Flag | Meaning |
|------|---------|
| `--cron` | Cron schedule. Empty means manual-only. |
| `--provider`, `--model` | Provider/model overrides (fall back to your defaults). |
| `--timeout` | Wall-clock budget as a Go duration (`15m`, `2h`). `0` means unlimited. |
| `--token-budget` | Estimated token cap. `0` means unlimited. |
| `--max-turns`, `--max-tool-calls` | Step budgets for the run. |
| `--tools` | Explicit list of enabled tools. **Omitting it disables all external-facing tools (web search, MCP) by default.** Naming a tool opts it back in. |
| `--deny` | Routine-scoped deny rules, applied at the highest priority. |
| `--workdir` | Working directory for the run. |
| `--disabled` | Create the routine without enabling it. |

## How routines run

Routines run unattended:

- **Permissions** use full auto-approve with explicit `deny` rules still honored. A routine
  cannot stop to ask you to confirm a tool; anything matching a `deny` rule is blocked.
- **Clarifications** never block: if the agent asks a question, it receives a standard
  "proceed using your best judgment" answer so the run does not deadlock.
- **External-facing tools** (web search and MCP tools) are disabled unless the routine's
  `--tools` list names them.
- **Budgets** stop the run cleanly: exceeding the time or token budget ends the run with a
  distinct status that is recorded and surfaced.
- **Model failure quits**: if the model/provider errors, the run stops, the error is recorded,
  and it is shown to you the next time you launch Terminal Agent.

Defaults for time (15m), tokens (1,000,000), provider/model, and step limits come from the
`routines` block in your configuration (see [Configuration](../configuration.md)).

## Results

Each run writes:

- a full JSONL transcript and a human-readable `.md` summary under
  `~/.local/share/terminal-agent/routines/<id>/logs/`,
- a status entry (active / inactive / error, last run, tokens used) used by `routine list`.

The next time you open Terminal Agent interactively, a one-line notice reports any routine
runs that completed since you were last here, highlighting failures.

```sh
agent routine run daily-standup        # run now and print the output
agent routine logs daily-standup --last # print the latest run summary
agent routine list                      # see status and recent activity
```

## Scheduling

A routine's `--cron` schedule is fired automatically by the [routine daemon](daemon.md).
Start it once with `agent daemon install` (or run `agent daemon start` in the foreground) and
enabled routines run on their schedules; `agent daemon status` shows what is scheduled and the
next fire time. You can always trigger a run yourself with `agent routine run <id>` regardless
of the daemon.
