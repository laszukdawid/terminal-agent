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

### Viewing Current Configuration

```sh
# View all settings
agent config get all

# View specific setting
agent config get provider
```

## Environment Variables

Terminal Agent uses environment variables for API keys:

| Provider | Environment Variable | Description |
|----------|---------------------|-------------|
| OpenAI | `OPENAI_API_KEY` | API key for OpenAI services |
| Anthropic | `ANTHROPIC_API_KEY` | API key for Anthropic Claude models |
| Perplexity | `PERPLEXITY_KEY` | API key for Perplexity AI |
| Google | `GOOGLE_API_KEY` | API key for Google AI (Gemini) |
| AWS Bedrock | AWS credentials | Standard AWS credential configuration |

Example of setting an environment variable:

```sh
export OPENAI_API_KEY=your_api_key_here
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
```

To see all available tasks:

```sh
task --list
```
