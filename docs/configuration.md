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

Terminal Agent supports tool execution permissions via the `permissions` key in configuration files. Permissions are evaluated using the same action expression format used by confirmations, for example:

```
unix("aws sso login", profile="dev")
```

### Global vs Local Configuration

- Global config: `$HOME/.config/terminal-agent/config.json`
- Local config: `.terminal-agent.json` files discovered by walking from the current working directory up to the filesystem root.

Local configs take priority over global rules when both match. The closest `.terminal-agent.json` to the current directory has the highest priority.

### Permissions Schema

```json
{
  "permissions": {
    "allow": ["unix(\"aws sso login\")"],
    "deny": ["unix(\"rm -rf .*\")"],
    "ask": ["unix(\"aws .*\")"]
  }
}
```

Rules are evaluated in this order:

1. `ask` matches always prompt, even if `allow` or `deny` also match.
2. Between `allow` and `deny`, the highest priority match wins; if both match at the same priority, `deny` wins.

### Confirmation Shortcuts

When prompted to execute an action, you can respond with:

- `y` / `yes` to allow once
- `n` / `no` to deny once
- `yes!` to allow and remember (writes to the nearest `.terminal-agent.json`, or global config if none)
- `no!` to deny and remember (writes to the nearest `.terminal-agent.json`, or global config if none)

## Environment Variables

Terminal Agent uses environment variables for API keys:

| Provider | Environment Variable | Description |
|----------|---------------------|-------------|
| OpenAI | `OPENAI_API_KEY` | API key for OpenAI services |
| Anthropic | `ANTHROPIC_API_KEY` | API key for Anthropic Claude models |
| Perplexity | `PERPLEXITY_KEY` | API key for Perplexity AI |
| Google | `GOOGLE_API_KEY` | API key for Google AI (Gemini) |
| AWS Bedrock | AWS credentials | Standard AWS credential configuration |
| Llama.cpp | `YZMA_LIB` | Path to the directory containing the local llama.cpp shared libraries used by the `llama` provider |
| Ollama | `OLLAMA_HOST` | Host URL for Ollama server |

Example of setting an environment variable:

```sh
export OPENAI_API_KEY=your_api_key_here
```

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

For persistent configuration, add this to your shell profile (`.bashrc`, `.zshrc`, etc.).

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

# Set provider to Perplexity
task run:set:perplexity

# Set provider to Ollama
task run:set:ollama
```

If you use the direct local `llama` provider, configure it manually through `agent config set` plus the `llama_models` alias map in `config.json`. The repo also includes `task deps:llama:cpu`, `task deps:llama:vulkan`, `task deps:llama:rocm`, and `task run:set:llama` helpers.

To see all available tasks:

```sh
task --list
```
