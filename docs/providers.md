# Supported LLM Providers

Terminal Agent supports multiple LLM providers, giving you flexibility to choose the one that best meets your needs.

## Configuring Providers

You can set your preferred provider and model using the `config` command:

```sh
agent config set provider <provider-name>
agent config set model <model-id>
```

## Provider-Specific Setup

### OpenAI

**Setup:**
1. Create an account at [OpenAI](https://platform.openai.com/)
2. Generate an API key
3. Set the key as an environment variable:
   ```sh
   export OPENAI_API_KEY=your_api_key_here
   ```

**Configuration:**
```sh
agent config set provider openai
agent config set model gpt-4o-mini
```

**Recommended Models:**
- `gpt-4o-mini` - Good balance of capability and cost
- `gpt-4o` - Higher capability, higher cost
- `gpt-3.5-turbo` - Faster, less capable

**Special Features:**
- Supports streaming output with the `--stream` flag
- Tool usage capability for the `task` command

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

**Configuration:**
```sh
agent config set provider bedrock
agent config set model anthropic.claude-3-haiku-20240307-v1:0
```

**Recommended Models:**
- `anthropic.claude-3-haiku-20240307-v1:0` - Good balance of capability and cost
- `anthropic.claude-3-sonnet-20240229-v1:0` - Higher capability
- `meta.llama3-8b-instruct-v1:0` - Open source alternative

**Special Features:**
- Access to multiple model families through one provider
- Tool usage capability for the `task` command
- Supports streaming output with the `--stream` flag

### Perplexity

**Setup:**
1. Create an account at [Perplexity AI](https://www.perplexity.ai/)
2. Generate an API key
3. Set the key as an environment variable:
   ```sh
   export PERPLEXITY_KEY=your_api_key_here
   ```

**Configuration:**
```sh
agent config set provider perplexity
agent config set model llama-3-8b-instruct
```

**Recommended Models:**
- `llama-3-8b-instruct` - Compact open-source model
- `llama-3-70b-instruct` - More capable open-source model
- `llama-3.1-8b-instruct` - Updated version

**Limitations:**
- Does not support streaming output
- Does not support the `task` command's tool usage capability

### Google (Gemini)

**Setup:**
1. Get access to [Google AI Studio](https://ai.google.dev/)
2. Generate an API key
3. Set the key as an environment variable:
   ```sh
   export GOOGLE_API_KEY=your_api_key_here
   ```

**Configuration:**
```sh
agent config set provider google
agent config set model gemini-2.0-flash-lite
```

**Recommended Models:**
- `gemini-2.0-flash-lite` - Fast response times
- `gemini-2.0-pro` - More capable model

**Special Features:**
- Tool usage capability for the `task` command
- Good integration with web search capabilities

## Performance Considerations

Different providers excel at different tasks:

- **Complex reasoning**: Anthropic Claude models (direct or via Bedrock)
- **Speed and cost-efficiency**: OpenAI's GPT-3.5, Google's Gemini Flash models
- **Creative tasks**: OpenAI's GPT-4 series, Anthropic Claude Opus
- **Open-source options**: Llama models via Perplexity or Bedrock
