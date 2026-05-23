# Terminal Agent Overview

Terminal Agent is a CLI-first AI assistant that lets you collaborate with large language models directly from your shell. It wraps multiple provider APIs (OpenAI, Anthropic, Bedrock, Google, Ollama, etc.), and also supports direct local `llama.cpp`-based GGUF inference through the `llama` provider. It renders markdown responses with Glamour, and exposes commands like `agent ask` for general questions and `agent task` for planning and executing terminal workflows. Configuration lives in `~/.config/terminal-agent/config.json`, can be overridden via CLI flags, and supports provider/model switching as well as MCP tool definitions.

Tool execution permissions can be set in `~/.config/terminal-agent/config.json` and in project-local `.terminal-agent.json` files. Local configs are discovered by walking from the current working directory up to the filesystem root; the closest file wins when priorities overlap. Permissions use the same action expression format shown in confirmations (for example `unix("aws sso login", profile="dev")`) and support `allow`, `deny`, and `ask` lists. When prompted, `yes!` or `no!` will remember a decision by writing to the nearest `.terminal-agent.json`, falling back to the global config when no local file exists.

The repo is structured around the Go implementation of the binary (see `cmd/` and `internal/`), plus documentation in `docs/` that backs the published site. Installation can happen via Homebrew, downloading release archives, `go install`, or building from source with Taskfile tasks such as `task build`/`task install`.

## Architecture Notes

`internal/app` is the shared application-service layer used by both the CLI and the GUI.

- CLI commands in `internal/commands/` call `internal/app` service methods such as `Ask`, `AskEvents`, `Chat`, and `Task`.
- GUI code in `internal/gui/` also depends on the same `internal/app` service interface rather than talking directly to connectors or prompt helpers.
- Changes in `internal/app` should therefore be reviewed as cross-surface changes even when only one entrypoint appears to be affected.
- The `llama` provider is a direct local runtime, not an HTTP API integration. It resolves model aliases from config and loads GGUF files plus a local llama.cpp shared library inside the current process.

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

## Running Tests

Testing is orchestrated through [Taskfile](https://taskfile.dev/). Common flows:

- `task test` – full suite (unit + integration) and what CI runs before tagging a release.
- `task test:unit` – fast feedback loop for Go unit tests.
- `task test:integration` – spins up the Docker-backed integration environment.

Taskfile commands will install Go dependencies automatically, but if you need to drop into language-specific tooling remember the project preference: use `uv run python …` instead of bare `python`, and `bun …` instead of `node`. These help keep virtual environments and JavaScript runtimes consistent across contributors.

## Local Development Tips

- Install Task (`brew install go-task/tap/go-task` or follow upstream docs) and Docker before running integration tests.
- `task build` produces the `agent` binary; `task install` additionally copies it to `~/.local/bin/agent`.
- Configuration helpers live under `task run:set:*` (e.g., `task run:set:openai`) for quickly switching providers during manual testing.
- The direct local `llama` provider currently requires `YZMA_LIB` to point at the directory containing the llama.cpp shared libraries, plus `llama_models` aliases in `~/.config/terminal-agent/config.json`. The documented Linux install path uses llama.cpp runtime build `b9180`.

## System Prompt Context

Every request to an LLM includes a system prompt assembled from several layers (see `internal/agent/prompt.go`):

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
