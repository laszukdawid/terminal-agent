# Terminal Agent Overview

Terminal Agent is a CLI-first AI assistant that lets you collaborate with large language models directly from your shell. It wraps multiple provider APIs (OpenAI, Anthropic, Bedrock, Perplexity, Ollama, etc.), renders markdown responses with Glamour, and exposes commands like `agent ask` for general questions and `agent task` for planning and executing terminal workflows. Configuration lives in `~/.config/terminal-agent/config.json`, can be overridden via CLI flags, and supports provider/model switching as well as MCP tool definitions.

Tool execution permissions can be set in `~/.config/terminal-agent/config.json` and in project-local `.terminal-agent.json` files. Local configs are discovered by walking from the current working directory up to the filesystem root; the closest file wins when priorities overlap. Permissions use the same action expression format shown in confirmations (for example `unix("aws sso login", profile="dev")`) and support `allow`, `deny`, and `ask` lists. When prompted, `yes!` or `no!` will remember a decision by writing to the nearest `.terminal-agent.json`, falling back to the global config when no local file exists.

The repo is structured around the Go implementation of the binary (see `cmd/` and `internal/`), plus documentation in `docs/` that backs the published site. Installation can happen via Homebrew, downloading release archives, `go install`, or building from source with Taskfile tasks such as `task build`/`task install`.

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

For more context (features, provider setup, MCP configuration), refer to `README.md` and the docs site referenced there.
