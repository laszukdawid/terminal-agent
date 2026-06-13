# Approval Logic

Terminal Agent decides whether a task tool call can run automatically, must ask for confirmation, or is blocked. This page is the source of truth for that decision flow.

Approval applies to `agent task` tool calls. Informational commands such as `agent ask` do not run tools.

## Decision Inputs

Each tool call is converted into an action string such as:

```text
unix("aws sso login", profile="dev")
file_edit(path="README.md", operation="write")
```

Approval uses these inputs:

- One-time decisions cached for the current task run.
- Permission rules from global config, local `.terminal-agent.json` files, and CLI `--allow` flags.
- The task command's `--auto-approve` flag.
- The tool's permission category.
- Tool-specific default checks, such as parser-verified read-only Unix commands.

## Decision Order

Terminal Agent evaluates a tool call in this order:

1. Empty actions are allowed.
2. A cached decision for the same action in the current run is reused.
3. If `--auto-approve` is set, matching `allow` or `deny` rules are evaluated. A matching `deny` still blocks. If no `allow` or `deny` rule matches, the action is approved without prompting.
4. If a matching `ask` rule exists, Terminal Agent prompts.
5. Matching `allow` and `deny` rules are evaluated. The highest-priority rule wins; at the same priority, `deny` wins.
6. If no rule matches, the default tool policy decides whether the action can run without prompting.
7. If the default policy does not allow the action, Terminal Agent prompts.

The important consequence is that `deny` remains a hard block even with `--auto-approve`, while `ask` is a prompt preference that `--auto-approve` bypasses for the current run.

## Rule Sources And Priority

Permission rules come from these sources, ordered from lowest to highest priority:

1. Global config: `$HOME/.config/terminal-agent/config.json`.
2. Local config: `.terminal-agent.json` files discovered by walking from the current working directory up to the filesystem root. The closest file has the highest priority among local configs.
3. CLI `--allow` flags on `agent task`.

Between `allow` and `deny` matches at different priorities, the highest priority wins. At the same priority, `deny` wins.

`ask` rules are checked before `allow` and `deny` during normal execution, so a matching `ask` rule prompts even if an `allow` rule also matches. With `--auto-approve`, `ask` rules are bypassed.

## Default Tool Policy

When no permission rule decides the action, Terminal Agent falls back to the tool's permission category.

| Category | Tools | Default |
| --- | --- | --- |
| `read` | `read`, `file_search`, `websearch`, `final_answer`, `ask_user` | Allowed without prompting, except `file_search` prompts when the requested root is outside the current read scope |
| `write` | `file_edit` | Allowed without prompting only when the target path is inside the task workspace root or was explicitly approved earlier in the run |
| `execute` | `unix`, `python` | Prompts, except parser-verified read-only `unix` commands |
| undeclared | MCP tools and third-party tools without a declared category | Treated as `execute` and prompts |

Default policy is only a fallback. A matching `deny` rule blocks even a read tool, an in-workspace write, or a read-only Unix command.

Approving an out-of-root `file_search` adds that directory to the read scope for the current run only. Approving an out-of-root `file_edit` adds only that exact file path to the write scope for the current run only. Read-scope approvals do not grant write scope.

## Read-Only Unix Auto-Approval

`unix` is generally an execute tool, but some shell programs are safe enough to run without prompting. Terminal Agent uses `mvdan.cc/sh` to parse the shell command and auto-approves only commands that satisfy all of these conditions:

- The command parses as a shell program made only of approved statement types.
- Every command in the pipeline is in the read-only allowlist.
- Sequential commands joined with `;` or `&&` are each independently approved.
- `cd <dir>` is allowed only when `<dir>` is static, exists, and resolves inside the task root; later statements are checked from that new directory.
- Static `for ... in ...; do ...; done` loops are allowed only when the iteration list is static and the body is otherwise approved.
- Every word is static shell syntax: literals, normal single-quoted strings, or normal double-quoted strings with only static content.
- `echo` may use the active static `for` loop variable, e.g. `for i in 1 2 3; do echo "$i"; done`.
- There are no redirections, background execution, negation, coprocs, disown markers, unapproved shell control operators, command substitutions, process substitutions, parameter expansions outside the narrow static-loop `echo` case, arithmetic expansions, variable assignments, subshells, blocks, unbounded `while`/`until` loops, conditionals, or function declarations.
- Command-specific write-capable flags are absent.

Examples that run without prompting by default:

```sh
ls -la
find . -type f | sort | head -20
ls -la | grep go | wc -l
env FOO=bar
cd docs; find . -type f
cd docs && pwd && find . -type f
for i in 1 2 3; do echo "$i"; pwd; done
```

Examples that still prompt unless explicitly allowed or auto-approved:

```sh
find . -delete
find . -exec rm {} \;
ls > out.txt
cd /tmp; find .
ls && rm file
ls; rm file
cat <(rm file)
FOO=bar ls
cat file | tee out.txt
while pwd; do echo ok; done
```

The read-only Unix classifier is intentionally conservative. False negatives are acceptable: if a safe command is not recognized, Terminal Agent asks for confirmation instead of running it automatically.

## `--auto-approve`

`agent task --auto-approve` automatically approves confirmation prompts for the current run.

It does:

- Skip interactive confirmation prompts.
- Bypass matching `ask` rules.
- Apply only to the current task run.
- Cache approved actions only in memory for that run.

It does not:

- Override matching `deny` rules.
- Persist any permissions to config files.
- Change default policy for future runs.

Use `--allow` for a high-priority temporary allow rule. Use `yes!` in an interactive prompt to persist an allow rule to the nearest `.terminal-agent.json`, or to global config when no local config exists.

## Confirmation Shortcuts

When prompted, these responses are available:

- `y` / `yes`: allow this action once for the current run.
- `n` / `no`: deny this action once for the current run.
- `yes!`: allow and remember by writing a permission rule.
- `no!`: deny and remember by writing a permission rule.

Remembered prompt decisions are written to the closest discovered `.terminal-agent.json`. If no local config exists, they are written to the global config.

## Implementation Pointers

The main implementation points are:

- `internal/agent/confirmation.go`: rule matching and `--auto-approve` policy.
- `internal/agent/task.go`: task-time confirmation calls and default tool policy.
- `internal/agent/readonly_unix.go`: parser-backed read-only Unix classifier.
- `internal/config/permissions.go`: loading global and local permission rule sets.
