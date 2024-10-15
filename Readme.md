# Terminal agent

An LLM Agent to help you from and within the terminal.

## Providers supported

To change the provider (to `$PROVIDER`) and model (to `$MODEL`) do

```sh
$ terminal-agent config set provider $PROVIDER
$ terminal-agent config set model $MODEL
```

### (Amazon) Bedrock

Terminal Agent uses the Bedrock [Converse API](https://docs.aws.amazon.com/bedrock/latest/userguide/conversation-inference-call.html) and thus supports the same models. These include models from Anthropic (newer than the Instance v1) and Llama 3+ version.

To use these models you need to make sure that you have valid aws credentials set. We support having auth via local credentials or via SSO.

Setting `bedrock` provider with Anthropic Haiku model can be done via

```sh
$ terminal-agent config set provider bedrock
$ terminal-agent config set anthropic.claude-3-haiku-20240307-v1:0 
```

### Perplexity AI

Perplexity AI has an "in development" functionality to provide access to models via their API. To read more on this visit their (documentation)[https://docs.perplexity.ai/guides/getting-started].

To use `terminal-agent` with Perplexity AI, you need to set `PERPLEXITY_KEY` env variable with your individual key, and then set the provider to `perplexity`.

Setting `perplexity` provider with `llama-3.1-8b-instruct` model can be done via

```sh
$ terminal-agent config set provider bedrock
$ terminal-agent config set llama-3.1-8b-instruct
```

## Features

- [x] **ask**: Ask general questions via terminal
- [x] **task**: Ability to propose and execute a command
- [ ] **unix**: Designs and evaluates a unix command
- [ ] **python**: Designs and evaluates a python command
- [ ] **git**: Designs and evaluates a git command

## Philosphy

```
The agent is not you.
You interact with the agent.
The agent knows about the tools.
The agent can use tools.
Tools are independent from the agent.
```

## Configuration

Each request can have its own configuration, and there are defaults provided.
To see default execute `terminal-agent config get all`.

To override defaults, use `config set` command, e.g.

```sh
$ terminal-agent config set model ai21.jamba-1-5-mini-v1:0
```

The config is stored in json file at `$HOME/.config/terminal-agent/config.json`.

## Development

### Setup

* Taskfile, instead of Makefile. See [Taskfile installation](https://taskfile.dev/installation/)
* Docker, needed for running integ tests
