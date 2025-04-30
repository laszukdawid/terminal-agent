# Config Command

The `config` command allows you to view and modify Terminal Agent's configuration settings.

## Usage

```sh
agent config <subcommand> [args]
```

## Subcommands

### get

Get the value of a specific configuration setting:

```sh
agent config get <key>
```

### get all

Get all configuration settings:

```sh
agent config get all
```

### set

Set a configuration value:

```sh
agent config set <key> <value>
```

## Configuration Keys

| Key | Description | Example Values |
|-----|-------------|---------------|
| `provider` | The LLM provider to use | `openai`, `anthropic`, `bedrock`, `perplexity`, `google` |
| `model` | The model ID to use | `gpt-4o-mini`, `claude-3-haiku-20240307`, etc. |
| `mcp-path` | Path to MCP JSON file | `/path/to/mcp.json` |

## Examples

```sh
# Set the provider to OpenAI
agent config set provider openai

# Set the model to GPT-4o Mini
agent config set model gpt-4o-mini

# Check the current provider
agent config get provider

# Check the current model
agent config get model

# Set the MCP file path
agent config set mcp-path /home/user/mcp.json

# View all configuration settings
agent config get all
```

## Configuration File

The configuration is stored in a JSON file at:

```
$HOME/.config/terminal-agent/config.json
```

While you can edit this file directly, it's recommended to use the `config` command to ensure proper formatting.

## Provider-Specific Configuration

Different providers may require different model IDs. Here are some common examples:

### OpenAI
```sh
agent config set provider openai
agent config set model gpt-4o-mini
```

### Anthropic
```sh
agent config set provider anthropic
agent config set model claude-3-5-sonnet-latest
```

### Amazon Bedrock
```sh
agent config set provider bedrock
agent config set model anthropic.claude-3-haiku-20240307-v1:0
```

### Perplexity
```sh
agent config set provider perplexity
agent config set model llama-3-8b-instruct
```

### Google
```sh
agent config set provider google
agent config set model gemini-2.0-flash-lite
```
