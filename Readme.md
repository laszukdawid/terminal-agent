# Terminal agent

An LLM Agent to help you from and within the terminal.

## Usage

### Recommended (download)

Recommended way of using the `terminal-agent` is to download the `agent` binary file from [Releases](https://github.com/laszukdawid/terminal-agent/releases). 
(Remember to set execution permissions with `chmod u+x agent`.)
To test the executability type `agent --help`.

### Compile source

Alternatively, you can compile the code yourself.
To do so, make sure you have golang installed for compilation and we recommend using [Taskfile](https://taskfile.dev/installation/) for task execution.
If you only want to compile the repo then execute `task build`.
In case you want to both build and set it in your path then execute `task install` which will put binary into `~/.local/bin/agent`.

For majority of cases, do check out the `Taskfile.dist.yaml` as it has most relevant tasks / receipies.

### Ask

Intended for general questions.

```sh
agent ask what is a file descriptor?
```

If you like to see characters appearing in terminal, add `--stream` flag. By default, all results are formatted as a markdown using [glamour](https://github.com/charmbracelet/glamour).

**Life hack**: If you set alias `alias aa="agent ask"` you'll have a quick shortcut to ask questions from terminal. It's quicker than opening browser to search! Execute `task install:alias` for auto-setup.

### Task

Generates code for execution, and asks for execution permissions.

```sh
agent task list files 3 dirs up
```

**Note**: Since more testing is required, there are only a few methods allowed.

### Config

Given that a few providers and their models are supported, there's a rudamentry configuration support. Whole config 

## Providers supported

To change the provider (to `$PROVIDER`) and model (to `$MODEL`) do

```sh
$ agent config set provider $PROVIDER
$ agent config set model $MODEL
```

### Anthropic

Anthropic is best known for their Sonnet and Haiku models. From subjective experience, they're one of the best commercially available models. One can use them either directly, or via [Amazon Bedrock](#amazon-bedrock).

To use directly, check out their [Getting Started](https://docs.anthropic.com/en/api/getting-started#accessing-the-api) page. Primarely what you need is to create an account, then create an API key, and set it as your env variable, e.g.

```sh
export ANTHROPIC_API_KEY=sk-ant-api00-something-something
```

Once you have the key set, then configure the terminal-agent to use your preferred provider and model. To see all available models, check [User Guide -> Models](https://docs.anthropic.com/en/docs/about-claude/models).

Setting the provider can be done using `task run:set:anthropic` or directly
```sh
$ agent config set provider anthropic
$ agent config set anthropic.claude-3-haiku-20240307
```

### (Amazon) Bedrock

Terminal Agent uses the Bedrock [Converse API](https://docs.aws.amazon.com/bedrock/latest/userguide/conversation-inference-call.html) and thus supports the same models. These include models from Anthropic (newer than the Instance v1) and Llama 3+ version.

To use these models you need to make sure that you have valid aws credentials set. We support having auth via local credentials or via SSO.

Setting `bedrock` provider with Anthropic Haiku model can be done with `task run:set:bedrock` or directly

```sh
$ agent config set provider bedrock
$ agent config set anthropic.claude-3-haiku-20240307-v1:0 
```


### Perplexity AI

Perplexity AI has an "in development" functionality to provide access to models via their API. To read more on this visit their (documentation)[https://docs.perplexity.ai/guides/getting-started].

To use `agent` with Perplexity AI, you need to set `PERPLEXITY_KEY` env variable with your individual key, and then set the provider to `perplexity`.

Setting `perplexity` provider with `llama-3.1-8b-instruct` model can be done with `task run:set:perplexity` or directly

```sh
$ agent config set provider perplexity
$ agent config set llama-3.1-8b-instruct
```

### OpenAI

To use `agent` with OpenAI, you need to set the `OPENAI_API_KEY` environment variable with your individual key, and then set the provider to `openai`.

Setting `openai` provider with `gpt-4o-mini` model can be done with `task run:set:openai` or directly

```sh
$ agent config set provider openai
$ agent config set model gpt-4o-mini
```

## Features

- [x] **ask**: Ask general questions via terminal (all models)
- [x] **task**: Ability to propose and execute a command (expect Perplexity)
- [x] **stream**: Ask only (except Perplexity)
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
To see default execute `agent config get all`.

To override defaults, use `config set` command, e.g.

```sh
$ agent config set model ai21.jamba-1-5-mini-v1:0
```

The config is stored in json file at `$HOME/.config/terminal-agent/config.json`.

## Development

### Setup

* Taskfile, instead of Makefile. See [Taskfile installation](https://taskfile.dev/installation/)
* Docker, needed for running integ tests
