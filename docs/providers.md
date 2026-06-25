# Supported LLM Providers

Terminal Agent supports multiple LLM providers, giving you flexibility to choose the one that best meets your needs.

## Configuring Providers

You can set your preferred provider and model using the `config` command:

```sh
agent config set provider <provider-name>
agent config set model <model-id>
```

## Provider-Specific Setup

### Llama.cpp

**Setup:**
1. Obtain a local GGUF model file.
2. Install llama.cpp shared libraries compatible with your machine.
3. Set the runtime library path:
   ```sh
   export YZMA_LIB=/absolute/path/to/libdir
   ```
4. Add a model alias to `~/.config/terminal-agent/config.json` under `llama_models`.

Example Linux CPU setup:

```sh
go install github.com/hybridgroup/yzma@v1.14.1
mkdir -p ~/.local/share/yzma/lib
~/go/bin/yzma install --lib ~/.local/share/yzma/lib --processor cpu --version b9180
export YZMA_LIB=$HOME/.local/share/yzma/lib
```

There are matching Taskfile helpers:

```sh
task deps:llama
task run:set:llama
```

**Configuration:**
```sh
agent config set provider llama
agent config set model llama3.2
agent config set device cpu
```

Example config:

```json
{
  "default_provider": "llama",
  "providers": {
    "llama": "llama3.2"
  },
  "llama_models": {
    "llama3.2": "/absolute/path/to/llama3.2.gguf"
  }
}
```

**Special Features:**
- Direct local inference without a separate HTTP server
- Supports streaming output with the `--stream` flag
- Uses the model's chat template when available
- Supports runtime device selection with `--device auto|cpu|gpu` and `agent config set device ...`
- Supports `task` through an agent-managed structured fallback when native tool calling is unavailable in the local runtime
- Reuses the loaded model across repeated turns within a single `task` run to avoid repeated model loads

**Limitations:**
- Requires `YZMA_LIB` to point at the directory containing local llama.cpp shared libraries
- Requires local GGUF model files and alias configuration
- The direct local runtime still does not expose provider-native structured tool calling
- `task` support depends on the selected model following the structured JSON action protocol, so it can be less reliable than providers with native tool APIs
- The documented Linux install path uses llama.cpp runtime build `b9180`

`--device` and the `device` config key affect only the direct `llama` provider. They do not change `ollama` or remote provider execution.

### OpenAI

The `openai` provider uses API-key authentication for OpenAI API services. Use the separate `codex` provider for OAuth-backed ChatGPT/Codex access.

**Option 1: API key via environment variable**

1. Create an account at [OpenAI](https://platform.openai.com/)
2. Generate an API key
3. Set the key as an environment variable:
   ```sh
   export OPENAI_API_KEY=your_api_key_here
   ```

**Option 2: Stored API key via agent auth**

Store your API key:
```sh
agent auth login openai --api-key
```

Credentials are persisted in `~/.config/terminal-agent/auth.json`. They are used automatically for `ask`, `chat`, and `task` commands when no `OPENAI_API_KEY` environment variable is present.

Check your auth status at any time:
```sh
agent auth status openai
```

Remove stored credentials:
```sh
agent auth logout openai
```

**Configuration:**
```sh
agent config set provider openai
agent config set model gpt-4o-mini
```

**Custom API Endpoints:**
If you're using an OpenAI-compatible API endpoint (e.g., Azure OpenAI, local LLM servers), you can set a custom base URL:
```sh
export OPENAI_BASE_URL=https://your-custom-endpoint.com/v1
```

**Recommended Models:**
- `gpt-4o-mini` - Good balance of capability and cost
- `gpt-4o` - Higher capability, higher cost
- `gpt-3.5-turbo` - Faster, less capable

**Special Features:**
- Supports streaming output with the `--stream` flag
- Tool usage capability for the `task` command
- Compatible with OpenAI-compatible endpoints via `OPENAI_BASE_URL`

**Auth resolution order:** When both a stored OpenAI API key and `OPENAI_API_KEY` exist, the environment variable takes precedence.

### Codex

The `codex` provider uses OAuth-backed ChatGPT/Codex access through `agent auth`.

Authenticate with your ChatGPT subscription:
```sh
agent auth login codex               # browser OAuth login
agent auth login codex --device      # terminal-friendly device login
```

Stored OAuth tokens refresh automatically while a valid refresh token is still available. If the browser callback does not complete automatically, `agent auth login codex` also accepts a pasted authorization code or full redirect URL as a fallback.

Check your auth status at any time:
```sh
agent auth status codex
```

Remove stored credentials:
```sh
agent auth logout codex
```

**Configuration:**
```sh
agent config set provider codex
agent config set model gpt-5-mini
```

Legacy OAuth credentials previously stored under `openai` are migrated to `codex` automatically on successful use.

### Xiaomi MiMo

MiMo uses an OpenAI-compatible chat completions API.

**Setup:**
1. Create an account at the [Xiaomi MiMo Open Platform](https://platform.xiaomimimo.com/)
2. Generate an API key
3. Set the key as an environment variable:
   ```sh
   export MIMO_API_KEY=your_api_key_here
   ```

**Configuration:**
```sh
agent config set provider mimo
agent config set model mimo-v2.5-pro
```

**Custom API Endpoints:**
If you need to override the default endpoint, set `MIMO_BASE_URL`:
```sh
export MIMO_BASE_URL=https://api.xiaomimimo.com/v1
```

**Recommended Models:**
- `mimo-v2.5-pro` - Default MiMo model

**Special Features:**
- Uses the OpenAI-compatible chat completions protocol
- Supports streaming output with the `--stream` flag
- Tool usage capability for the `task` command when supported by the selected MiMo model

### Anthropic

**Setup:**
1. Create an account at [Anthropic](https://www.anthropic.com/)
2. Generate an API key from the [Console](https://console.anthropic.com/)
3. Set the key as an environment variable:
   ```sh
   export ANTHROPIC_API_KEY=your_api_key_here
   ```

**Configuration:**
```sh
agent config set provider anthropic
agent config set model claude-3-5-sonnet-latest
```

**Recommended Models:**
- `claude-3-5-sonnet-latest` - High capability
- `claude-3-haiku-20240307` - Faster, more economical
- `claude-3-opus-20240229` - Highest capability

**Special Features:**
- Excellent at complex reasoning tasks
- Tool usage capability for the `task` command
- Supports streaming output with the `--stream` flag

### Google (Gemini)

**Setup:**
1. Get access to [Google AI Studio](https://ai.google.dev/)
2. Generate an API key
3. Set the key as an environment variable:
   ```sh
   export GEMINI_API_KEY=your_api_key_here
   ```

**Configuration:**
```sh
agent config set provider google
agent config set model gemini-3.1-flash-lite
```

**Recommended Models:**
- `gemini-3.1-flash-lite` - Fast response times
- `gemini-2.0-pro` - More capable model

**Special Features:**
- Tool usage capability for the `task` command
- Good integration with web search capabilities

### Ollama

**Setup**:
1. Make sure you have [Ollama](https://ollama.com/) installed
2. Make sure you downloaded the model you'd like to use, e.g. `ollama pull llama3.2`

**Configuration:**
```sh
agent config set provider ollama
agent config set model llama3.2
```

In case your Ollama serve is running on a non-default server you can set the URL via env variable, i.e.
```sh
export OLLAMA_HOST=http://localhost:11345  # modify
```

**Special Features:**
- Supports streaming output with the `--stream` flag
- Supports tool usage for the `task` command

### Amazon Bedrock

**Setup:**
1. Set up an [AWS account](https://aws.amazon.com/) with Bedrock access
2. Configure your AWS credentials as usual:
   ```sh
   # Either through AWS CLI:
   aws configure
   
   # Or through environment variables:
   export AWS_ACCESS_KEY_ID=your_access_key
   export AWS_SECRET_ACCESS_KEY=your_secret_key
   export AWS_REGION=your_region
   ```

   SSO profiles are supported through the standard AWS SDK credential chain:
   ```sh
   aws configure sso --profile dev
   aws sso login --profile dev

   AWS_PROFILE=dev agent ask --provider bedrock "Hello from Bedrock"
   ```

   If the selected AWS profile includes a region, no region environment variable is required. Otherwise set `AWS_REGION` or configure a Bedrock region in `~/.config/terminal-agent/config.json`.

**Configuration:**
```sh
agent config set provider bedrock
agent config set model zai.glm-4.7-flash
```

Optional persistent AWS profile/region settings can be added directly to `~/.config/terminal-agent/config.json`:

```json
{
  "bedrock": {
    "profile": "dev",
    "region": "us-west-2",
    "prices": {
      "us-west-2": {
        "zai.glm-4.7-flash": {
          "input_per_1k": 0.00007,
          "output_per_1k": 0.0004,
          "last_checked": "2026-06-09T00:00:00Z"
        }
      }
    }
  }
}
```

When these fields are omitted, Bedrock uses the normal AWS SDK resolution order, including `AWS_PROFILE`, `AWS_REGION`, `AWS_DEFAULT_REGION`, and shared AWS config files. If no region is resolved, Terminal Agent falls back to `us-east-1`.

When you set a Bedrock model with `agent config set model ...`, Terminal Agent checks the cached model price for the configured region. If the price is missing or `last_checked` is older than 24 hours, it makes a three-second AWS Price List API refresh request to update the cached `input_per_1k` and `output_per_1k` values. Runtime model calls do not query pricing APIs. If pricing refresh fails, the model is still saved, but cost estimates for that model and region are unavailable until pricing is configured or refreshed successfully.

**Recommended Models:**
- `zai.glm-4.7-flash` - Fast and cost-effective default
- `anthropic.claude-3-haiku-20240307-v1:0` - Good balance of capability and cost
- `anthropic.claude-3-sonnet-20240229-v1:0` - Higher capability
- `meta.llama3-8b-instruct-v1:0` - Open source alternative

**Special Features:**
- Access to multiple model families through one provider
- Tool usage capability for the `task` command
- Supports streaming output with the `--stream` flag

### Mistral

**Setup:**
1. Create an account at [Mistral AI](https://console.mistral.ai/)
2. Generate an API key from the [Console](https://console.mistral.ai/api-keys/)
3. Set the key as an environment variable:
   ```sh
   export MISTRAL_API_KEY=your_api_key_here
   ```

**Configuration:**
```sh
agent config set provider mistral
agent config set model mistral-small-latest
```

**Custom API Endpoints:**
If you're using a Mistral-compatible API endpoint, you can set a custom base URL:
```sh
export MISTRAL_BASE_URL=https://your-custom-endpoint.com
```

**Recommended Models:**
- `mistral-small-latest` - Good balance of capability and cost
- `mistral-large-latest` - Higher capability, higher cost
- `mistral-medium-latest` - Medium capability
- `codestral-latest` - Optimized for code generation
- `ministral-8b-latest` - Smaller, faster model

**Special Features:**
- Tool usage capability for the `task` command
- Supports streaming output with the `--stream` flag
- Compatible with Mistral-compatible endpoints via `MISTRAL_BASE_URL`

## Performance Considerations

Different providers excel at different tasks:

- **Complex reasoning**: Anthropic Claude models (direct or via Bedrock)
- **Speed and cost-efficiency**: OpenAI's GPT-3.5, Google's Gemini Flash models, Mistral Small models
- **Creative tasks**: OpenAI's GPT-4 series, Anthropic Claude Opus, Mistral Large models
- **Open-source options**: Llama models via the local `llama` provider, Ollama, or Bedrock-hosted Llama models
