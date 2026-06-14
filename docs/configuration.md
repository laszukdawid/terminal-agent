# Advanced Configuration

Terminal Agent offers various configuration options to customize its behavior.

## Configuration File

The configuration is stored in a JSON file at:

```
$HOME/.config/terminal-agent/config.json
```

While you can edit this file directly, it's recommended to use the `config` command to ensure proper formatting.

## Basic Configuration

### Setting Provider and Model

```sh
# Set your preferred provider
agent config set provider openai

# Set your preferred model
agent config set model gpt-4o-mini
```

### Setting Device Preference

Use `device` to control direct local `llama` inference placement. Valid values are `auto`, `cpu`, and `gpu`.

```sh
# Persist a CPU preference for direct llama runs
agent config set device cpu

# Override per command when needed
agent ask --device gpu "Explain this stack trace"
```

`--device` overrides the config value when provided. If neither is set, Terminal Agent uses `auto`.

The setting only affects the direct `llama` provider. It applies to direct local query runs and `task` executions for `llama`, and is ignored for `ollama` and network-backed providers.

### Llama Local Model Aliases

The `llama` provider resolves logical model names through a `llama_models` alias map in the main config file.

Example:

```json
{
  "default_provider": "llama",
  "providers": {
    "llama": "llama3.2"
  },
  "llama_models": {
    "llama3.2": "/absolute/path/to/llama3.2.gguf",
    "qwen2.5-coder": "/absolute/path/to/qwen2.5-coder.gguf"
  }
}
```

When `provider` is `llama`, the configured model name is resolved in this order:

1. exact file path if the configured model value already points to a readable local file
2. alias lookup in `llama_models`
3. clear runtime error if neither resolves

### Viewing Current Configuration

```sh
# View all settings
agent config get all

# View specific setting
agent config get provider
```

## Permissions

Terminal Agent supports tool execution permissions via the `permissions` key in configuration files and the `--allow` CLI flag. Permissions are evaluated using the same action expression format used by confirmations, for example:

```
unix("aws sso login", profile="dev")
```

For the full approval decision order, `--auto-approve` behavior, and default tool policy, see [Approval Logic](approval-logic.md).

### Permission Sources and Priority

Permission rules come from three sources, listed from lowest to highest priority:

1. Global config: `$HOME/.config/terminal-agent/config.json`
2. Local config: `.terminal-agent.json` files discovered by walking from the current working directory up to the filesystem root. The closest file to the current directory has the highest priority among local configs.
3. CLI `--allow` flag (task command only): the highest priority rule set. Use it to temporarily allow an action without modifying config files.

The task command also supports `--auto-approve`, which automatically approves confirmation prompts for the current run. It bypasses `ask` prompts but still respects `deny` rules.

Between `allow` and `deny` rules at different priorities, the highest priority match wins. At the same priority, `deny` wins over `allow`.

### Global vs Local Configuration

- Global config: `$HOME/.config/terminal-agent/config.json`
- Local config: `.terminal-agent.json` files discovered by walking from the current working directory up to the filesystem root.

Local configs take priority over global rules when both match. The closest `.terminal-agent.json` to the current directory has the highest priority.

An empty `.terminal-agent.json` file (or one containing only whitespace) is treated as having no permission rules and does not affect evaluation. You can create an empty file as a placeholder without changing behavior.

When `yes!` or `no!` writes a remembered decision, it writes to the closest discovered `.terminal-agent.json`. If no local config exists, it falls back to writing to the global `config.json`.

### Permissions Schema

```json
{
  "permissions": {
    "allow": ["unix(\"aws sso login\")"],
    "deny": ["unix(\"rm -rf *\")"],
    "ask": ["unix(\"aws *\")"]
  }
}
```

Within configured permission rules, matching works this way:

1. `ask` matches always prompt, even if `allow` or `deny` also match.
2. Between `allow` and `deny`, the highest priority match wins; if both match at the same priority, `deny` wins.

See [Approval Logic](approval-logic.md) for the full runtime order, including cached decisions, `--auto-approve`, and default tool policy.

### Action Expression Format

Permissions use an action expression syntax that mirrors how tool calls appear in confirmation prompts:

```
tool_name("positional value", key1="value1", key2="value2")
```

The first positional argument (typically the command string for `unix` tools) and named arguments (`key=value`) use glob matching against the full value.

Supported glob syntax:

- `*` matches any sequence of characters, including spaces and `/`
- `?` matches exactly one character
- `[abc]` matches one character from a set
- `[a-z]` matches one character from a range
- `[!abc]` negates a set
- `\` escapes the next character so you can match a literal `*`, `?`, `[` or `]`

Examples:

- `unix("aws *")` matches `unix("aws sso login")`
- `unix("ls -d ~/*/")` matches `unix("ls -d ~/Desktop/*/")`
- `unix("ls -d \\*/")` matches the literal command `ls -d */`
- `unix("aws login", region="us-*")` matches `region="us-west-2"`
- `unix("aws login", allowKeys=["region", "profile", "read*"])` allows only matching key names

The `final` field that certain tools support (`unix`, `python`, `file_search`) is ignored during permission matching. This means `unix("ls -la", final=true)` is treated identically to `unix("ls -la")` for allow/deny/ask purposes.

### Confirmation Shortcuts

When prompted to execute an action, you can respond with:

- `y` / `yes` to allow once
- `n` / `no` to deny once
- `yes!` to allow and remember (writes to the nearest `.terminal-agent.json`, or global config if none)
- `no!` to deny and remember (writes to the nearest `.terminal-agent.json`, or global config if none)

## Environment Variables

Terminal Agent supports two credential sources:

**Environment variables** and **stored credentials** in `~/.config/terminal-agent/auth.json`.

Stored credentials are managed with the `agent auth` command and take effect automatically when the corresponding environment variable is unset.

| Provider | Environment Variable | Auth File | Description |
|----------|---------------------|-----------|-------------|
| OpenAI | `OPENAI_API_KEY` | supported | API key for OpenAI API services |
| Codex | — | supported | OAuth login for ChatGPT/Codex services |
| Anthropic | `ANTHROPIC_API_KEY` | — | API key for Anthropic Claude models |
| Google | `GEMINI_API_KEY` | — | API key for Google AI (Gemini) |
| Xiaomi MiMo | `MIMO_API_KEY` | — | API key for Xiaomi MiMo models |
| Mistral | `MISTRAL_API_KEY` | — | API key for Mistral AI models |
| AWS Bedrock | AWS credentials | — | Standard AWS credential configuration |
| Llama.cpp | `YZMA_LIB` | — | Path to the directory containing the local llama.cpp shared libraries used by the `llama` provider |
| Ollama | `OLLAMA_HOST` | — | Host URL for Ollama server |

Example of setting an environment variable:

```sh
export OPENAI_API_KEY=your_api_key_here
```

Example of storing an API key with the auth command:

```sh
agent auth login openai --api-key
```

Example of using OAuth-based Codex login instead:

```sh
agent auth login codex          # browser OAuth login
agent auth login codex --device # device-code login
```

**Auth resolution order for OpenAI:** `OPENAI_API_KEY` env var → stored API key in auth.json → error.

**Auth resolution for Codex:** stored OAuth credential in auth.json → legacy OpenAI OAuth credential migration → error.

For the `llama` provider, example runtime setup is:

```sh
export YZMA_LIB=$HOME/.local/share/yzma/lib
```

Example Linux CPU runtime install flow:

```sh
go install github.com/hybridgroup/yzma@v1.14.1
mkdir -p ~/.local/share/yzma/lib
~/go/bin/yzma install --lib ~/.local/share/yzma/lib --processor cpu --version b9180
export YZMA_LIB=$HOME/.local/share/yzma/lib
```

For persistent CLI configuration, add this to your shell profile (`.bashrc`, `.zshrc`, etc.).

### Web Search

The `ask` command can use the built-in `websearch` tool to fetch up-to-date information before answering. It is enabled by default and relies on the [Tavily](https://tavily.com) API.

| Environment Variable | Description |
|----------------------|-------------|
| `TAVILY_KEY` | API key used by the `websearch` tool. Web search is skipped when it is unset. |

To control web search:

- Per run: pass `--websearch=false` to `agent ask` for a quicker answer with no search.
- Globally: set `"web_search": false` in `~/.config/terminal-agent/config.json` (the key defaults to `true` when omitted).

Web search additionally requires a provider that supports tool calling; the local `llama` provider does not, so `ask` answers without searching there. See [Ask Command](./commands/ask.md#web-search) for details.

### GUI Environment Loading

Desktop-launched GUI processes usually do not source shell startup files directly. On startup, `agent-gui` resolves supported environment variables in this order:

1. Existing process environment from the launcher/session
2. GUI env file at `~/.config/terminal-agent/.gui.env`
3. Interactive shell import, when enabled
4. Stored OpenAI auth, when no `OPENAI_API_KEY` is visible

Use `.gui.env` when you want app-specific tokens that are separate from personal shell tokens:

```sh
OPENAI_API_KEY=your_gui_api_key_here
GEMINI_API_KEY=your_gui_api_key_here
TAVILY_KEY=your_gui_api_key_here
```

The GUI env file supports simple `KEY=value` lines and optional `export KEY=value` lines. It is parsed as data, not executed as a shell script. Keep it private with `chmod 600 ~/.config/terminal-agent/.gui.env`.

Shell import is disabled by default because shell startup files can be slow or interactive. Enable it only if you want `agent-gui` to fill missing supported variables from your shell environment:

```json
{
  "gui": {
    "env_file": "~/.config/terminal-agent/.gui.env",
    "load_shell_environment": false,
    "shell_environment_timeout": "2s"
  }
}
```

The Settings dialog reports whether the GUI environment was loaded. Restart `agent-gui` after changing `.gui.env` or shell startup files.

### GUI Voice Input

The popup GUI supports focused-window voice input. Press `F1` while the input is focused, or click `Listen`, to start recording. Press `F1` again or click `Stop` to finish recording. The transcript is inserted into the input and submitted through the normal Ask path by default.

Voice input is enabled by default, but recording starts only after an explicit toggle. Raw audio is kept in memory and is not written to disk. With the default OpenAI speech-to-text backend, recorded audio is sent to OpenAI for transcription. The resulting transcript is treated like typed input and is included in normal Ask/session logging.

Configuration lives under the global `gui.voice` section of `~/.config/terminal-agent/config.json`:

```json
{
  "gui": {
    "voice": {
      "enabled": true,
      "trigger_key": "F1",
      "auto_submit": true,
      "max_recording_duration": "60s",
      "stt": {
        "backend": "openai",
        "model": "gpt-4o-mini-transcribe",
        "language": ""
      }
    }
  }
}
```

Supported first-pass behavior:

- The GUI window must be focused; there is no global hotkey capture.
- The trigger is toggle-based, not push-to-talk.
- `auto_submit=false` leaves the transcript in the input for review.
- OpenAI STT uses the existing OpenAI auth resolution: `OPENAI_API_KEY`, GUI env loading, or stored OpenAI API key auth.
- `OPENAI_BASE_URL` is respected for OpenAI-compatible endpoints.

## Model Context Protocol (MCP)

Terminal Agent supports the Model Context Protocol (MCP) for defining custom tools:

```sh
# Set path to your MCP configuration file
agent config set mcp-path /path/to/mcp.json
```

### MCP File Format

The MCP file should be a JSON file conforming to the MCP specification. Here's an example:

```json
{
  "name": "my-custom-tools",
  "version": "0.1",
  "tools": [
    {
      "name": "custom-search",
      "description": "Search for information",
      "input_schema": {
        "type": "object",
        "properties": {
          "query": {
            "type": "string",
            "description": "The search query"
          }
        },
        "required": ["query"]
      }
    }
  ]
}
```

## Logging Configuration

You can control the verbosity of Terminal Agent's logs with the `--loglevel` flag:

```sh
# For debugging information
agent --loglevel debug ask "What is a file descriptor?"

# For minimal output
agent --loglevel error ask "What is a file descriptor?"
```

Available log levels (from most to least verbose):
- `debug`
- `info` (default)
- `warn`
- `error`
- `dpanic`
- `panic`
- `fatal`

## History Configuration

Terminal Agent stores interaction history in:

```
$HOME/.local/share/terminal-agent/query_log.jsonl
```

To enable logging for a specific interaction, use the `--log` flag:

```sh
agent ask --log "What is a file descriptor?"
```

## Terminal Context Storage

When `bash-reader` is installed (`agent plugin install bash-reader`), terminal context data is stored in:

```
$HOME/.local/share/terminal-agent/terminal-context/
```

Primary files:

- `index.log` — command index used by `agent ask --use-terminal-context`
- `sessions/` — reserved for session-related context files

To remove plugin data completely:

```sh
agent plugin uninstall bash-reader --purge-data
```

## Using Taskfile for Convenience

Terminal Agent includes a comprehensive Taskfile with predefined tasks:

```sh
# Set provider to OpenAI
task run:set:openai

# Set provider to Anthropic
task run:set:anthropic

# Set provider to Bedrock
task run:set:bedrock

# Set provider to Google
task run:set:google

# Set provider to Ollama
task run:set:ollama
```

If you use the direct local `llama` provider, configure it manually through `agent config set` plus the `llama_models` alias map in `config.json`. The repo also includes `task deps:llama:cpu`, `task deps:llama:vulkan`, `task deps:llama:rocm`, and `task run:set:llama` helpers.

To see all available tasks:

```sh
task --list
```
