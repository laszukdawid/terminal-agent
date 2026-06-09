# Terminal Agent Overview

Terminal Agent is a CLI-first AI assistant that lets you collaborate with large language models directly from your shell. It wraps multiple provider APIs (OpenAI, Anthropic, Bedrock, Google, Ollama, etc.), and also supports direct local `llama.cpp`-based GGUF inference through the `llama` provider. It renders markdown responses with Glamour, and exposes commands like `agent ask` for general questions and `agent task` for planning and executing terminal workflows. Configuration lives in `~/.config/terminal-agent/config.json`, can be overridden via CLI flags, and supports provider/model switching as well as MCP tool definitions.

Tool execution permissions can be set in `~/.config/terminal-agent/config.json` and in project-local `.terminal-agent.json` files. Local configs are discovered by walking from the current working directory up to the filesystem root; the closest file wins when priorities overlap. Permissions use the same action expression format shown in confirmations (for example `unix("aws sso login", profile="dev")`) and support `allow`, `deny`, and `ask` lists. When prompted, `yes!` or `no!` will remember a decision by writing to the nearest `.terminal-agent.json`, falling back to the global config when no local file exists.

Whether a tool prompts by default (when no `allow`/`deny`/`ask` rule matches) is driven by a per-tool **permission category** rather than a hardcoded list. Tools declare their category via the optional `tools.CategorizedTool` interface (`PermissionCategory()` returning `read`, `write`, or `execute`):

- **read** (`read`, `file_search`, `websearch`, `ask_user`, `final_answer`) — never prompts.
- **write** (`file_edit`) — does not prompt when the target path resolves inside the task workspace root; prompts otherwise. `file_edit` is already physically confined to the root by `ensureWithinRoot`, so the prompt branch is a backstop for future write tools.
- **execute** (`unix`, `python`) — always prompts; arbitrary effect.
- **undeclared** (MCP tools, any new tool not implementing the interface) — treated as `execute` and gated by default. This is deliberate: a tool with no declared category must not run silently.

The `allow`/`deny`/`ask` rule engine is unchanged and remains the override layer on top of these category defaults: `ask` always prompts, `deny` beats `allow` at equal-or-higher priority, and matched decisions are cached per run. The category only decides the fallback when no rule matches. See `permissionCategoryFor`/`confirmTool` in `internal/agent/task.go` and `ConfirmWithDefault` in `internal/agent/confirmation.go`.

The repo is structured around the Go implementation of the binary (see `cmd/` and `internal/`), plus documentation in `docs/` that backs the published site. Installation can happen via Homebrew, downloading release archives, `go install`, or building from source with Taskfile tasks such as `task build`/`task install`.

## Contribution Conventions

- Do **not** include design specs or implementation plans in pull requests or git commits. Keep them local only (e.g. under `docs/superpowers/`, which is git-ignored); a PR should contain only the production code, tests, and user-facing docs for the change.
- Do **not** commit build artifacts (the `agent` / `agent-gui` binaries produced by `task build`). They are git-ignored; never `git add` them into a commit.
- Prefer named constants over magic strings and numbers. If a value has product meaning, is reused, or is part of configuration/default behavior, define it as a const and reference the const in production code and tests.
- Keep source files focused. Do not let files grow into catch-all buckets; when a file starts mixing separate responsibilities, split it along durable product or implementation boundaries.
- Periodically revisit filenames as code evolves. A file name should still describe what the file holds today, not only the original intent from when it was created.

## Development Philosophy

- Optimize for the best durable solution, not the smallest or quickest implementation path. Prefer designs that make the idea clear, robust, and future-proof even when they require more code.
- Avoid shortcuts, temporary workarounds, and narrowly scoped fixes unless the user explicitly asks for them. If a shortcut is unavoidable, call it out clearly and explain the tradeoff.
- Treat coding as the easy part. Spend the necessary effort on understanding the product idea, long-term architecture, edge cases, and future maintenance before choosing an implementation.

## Architecture Notes

`internal/app` is the shared application-service layer used by both the CLI and the GUI.

- CLI commands in `internal/commands/` call `internal/app` service methods such as `Ask`, `AskEvents`, `Chat`, and `TaskEvents`.
- GUI code in `internal/gui/` also depends on the same `internal/app` service interface rather than talking directly to connectors or prompt helpers.
- Changes in `internal/app` should therefore be reviewed as cross-surface changes even when only one entrypoint appears to be affected.
- The `llama` provider is a direct local runtime, not an HTTP API integration. It resolves model aliases from config and loads GGUF files plus a local llama.cpp shared library inside the current process.
- Task execution is event-driven at the app layer. `internal/agent` orchestrates task steps and tool use, while confirmation and clarification transport is handled outside the agent core.

Task tool output has two separate paths:

- The tool's returned string is the captured result used by the agent for history, reasoning, summaries, and `final=true` direct output.
- `tools.ToolExecutionContext.Output` is an optional live display sink for incremental user-facing output while a tool is still running. Tools may ignore it, but process-style tools such as `unix` and `python` should write stdout/stderr chunks there as soon as they are available while still returning the captured output at completion.

For task events, live tool output is emitted as `EventOutputDelta` with `ToolName` and, when available, `ProcessID`. CLI printing is controlled by the task `--print` flag: `--print=false` suppresses both final output and live tool output, but the stream is still consumed so the task can continue. If live output delivery fails without task cancellation, the process should keep running and the app should emit `EventWarning` with the process id when the event channel is still usable; if the event path itself is broken, log the warning instead. Do not report display failures as command execution failures unless the context is canceled.

Prompt and runtime behavior is also shared through this layer, with one important distinction:

- `ask` and `chat` use ask-prompt resolution and may include memory/context features.
- `task` should resolve only the task prompt it needs.
- Do not couple task execution to ask-prompt resolution or ask-only features, because a broken ask prompt should not block task execution.
- The current `llama` provider supports direct local query generation for `ask`/GUI flows, but not `task` tool calling yet.

Task execution has a direct-output path for output-oriented tools:

- `unix`, `python`, and `file_search` support an optional boolean tool argument `final`.
- When `final=true` and the tool succeeds, the task agent should return that raw tool output immediately instead of doing another LLM summarization round.
- Use this for requests where the tool output itself is the answer, such as listing files or printing search results.
- Permission matching should ignore the `final` field so `unix("ls -la", final=true)` is treated like `unix("ls -la")` for allow/deny purposes.

## Session Logging

Every `ask`, `chat`, and `task` run writes an always-on, file-only execution log (one JSONL file per run) under `~/.local/share/terminal-agent/sessions/`, named `{timestamp}_{kind}_{shortid}.jsonl`. See `internal/sessionlog` (the `Recorder`, `Meta` header, and unified `Record` schema) and the inline `recorder.Write` calls in each app-layer flow.

When debugging a recent or current run, inspect the relevant JSONL session log in `~/.local/share/terminal-agent/sessions/` directly instead of asking the user to provide logs. Use the newest matching `{timestamp}_{kind}_{shortid}.jsonl` file unless the user points to a specific run.

- The recorder is created per run at the app boundary (`AskEvents`/`ChatEvents`/`TaskEvents`) and writes a `meta` provenance header first (`user`, `cwd`, `provider`, `model`, `command`, `created_at` — intentionally no hostname).
- For `ask`/`chat`, recording is inlined at each call site (request, completed, failed). For `task`, app-layer records are also inlined, while agent-internal steps (thoughts, tool calls, results) are captured via an `OnStep` callback in `TaskOptions` that writes through `taskStepToRecord`.
- `taskExecutionState.appendStep` is the single chokepoint that both appends to history (for prompt building) and forwards the enriched step to the `OnStep` callback.
- Logging must never break a run: `Recorder.Write` swallows errors (warn-log only) and is mutex-guarded for concurrent writers and monotonic `seq`.
- Tests redirect output via the `TERMINAL_AGENT_SESSIONS_DIR` env var (see `internal/app/SessionDir` and the app package `TestMain`) to avoid polluting the real data directory.

## Running Tests

Testing is orchestrated through [Taskfile](https://taskfile.dev/). Common flows:

- `task test` – full suite (unit + integration) and what CI runs before tagging a release.
- `task test:unit` – fast feedback loop for Go unit tests.
- `task test:integration` – spins up the Docker-backed integration environment.

Test code should prefer table-driven tests: define test case tables for related scenarios and iterate with `t.Run`. Use standalone test functions only when the setup or assertion flow is genuinely unique enough that a table would make the test less clear.

Taskfile commands will install Go dependencies automatically, but if you need to drop into language-specific tooling remember the project preference: use `uv run python …` instead of bare `python`, and `bun …` instead of `node`. These help keep virtual environments and JavaScript runtimes consistent across contributors.

Documentation is built with MkDocs Material. If `mkdocs` is installed locally, use `mkdocs build --strict`. If it is not installed, use `uvx --with mkdocs-material mkdocs build --strict`; plain `uvx mkdocs` is insufficient because it does not include the `material` theme. If strict mode fails on pre-existing warnings unrelated to your change, run `uvx --with mkdocs-material mkdocs build` to confirm the site still builds, note the strict warnings, and remove the generated `site/` directory afterward.

## Local Development Tips

- Install Task (`brew install go-task/tap/go-task` or follow upstream docs) and Docker before running integration tests.
- `task build` produces the `agent` binary; `task install` additionally copies it to `~/.local/bin/agent`.
- Configuration helpers live under `task run:set:*` (e.g., `task run:set:openai`) for quickly switching providers during manual testing.
- The direct local `llama` provider currently requires `YZMA_LIB` to point at the directory containing the llama.cpp shared libraries, plus `llama_models` aliases in `~/.config/terminal-agent/config.json`. The documented Linux install path uses llama.cpp runtime build `b9180`.

## System Prompt Context

Every request to an LLM includes a system prompt assembled from several layers (see `internal/agent/prompt.go`):

Prompt-building code should prefer templates with named variables over positional string builders or `fmt.Sprintf` placeholders. Named fields such as `{{.OriginalQuery}}`, `{{.CurrentDir}}`, or `{{.History}}` make prompt changes easier to review and reduce accidental argument-order bugs. Keep template data structs small and specific to the prompt being rendered.

1. **Header (`{{header}}`)** — `SystemPromptHeader()` injects dynamic system information:
   - Hostname, username, current time
   - Working directory
   - Operating system, architecture, and OS version (read from `/etc/os-release` on Linux or `sw_vers` on macOS)

2. **Role prompt** — command-specific instructions appended after the header:
   - `SystemPromptAsk` (for `agent ask`) — no-tools guidance, refers users to the `task` command for actions.
   - `SystemPromptTask` (for `agent task`) — agentic instructions with tool-use guidance.

3. **Custom prompts** — can override the default role prompt via:
   - `--prompt` CLI flag (highest priority)
   - File-based prompt in the configured working directory (`ask/system.prompt` or `task/system.prompt`)
   - All custom prompts still support `{{header}}` substitution.

4. **Memory** (ask only) — when enabled via config (`memory: true`) or the `--memory`/`-M` flag, stored memory entries from `~/.config/terminal-agent/memory.jsonl` are formatted as `<memory>…</memory>` and prepended to the system prompt. See `internal/memory/main.go` and `internal/commands/ask_cmd.go:BuildAskPrompt`.

5. **Context files** (ask only) — the `--context`/`-c` flag reads files and wraps each in `<context>…</context>` tags, prepended to the **user message** (not the system prompt).

For more context (features, provider setup, MCP configuration), refer to `README.md` and the docs site referenced there.
