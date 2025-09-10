# Terminal agent

An LLM Agent to help you from and within the terminal.

Read more https://laszukdawid.github.io/terminal-agent/.

## Example

<img src="./docs/assets/aa-how-to-attach-image.png" width="500" alt="answer to how to attach an image to Readme" />

<img src="./docs/assets/stream-example.gif" width="500" alt="example of streaming in terminal" />

## Installation

### Option 1: Download pre-built binary (Recommended for all)

Download the `agent-*-*` binary file from [Releases](https://github.com/laszukdawid/terminal-agent/releases). If you're on Linux you likely want `agent-linux-amd64` and if you're on MacOS it's likely `agent-macos-arm64`. After downloading, rename it to `agent` and add it to your path with binaries.
(Remember to set execution permissions with `chmod u+x agent`.)
To test the executability type `agent --help`.


### Option 2: go install (Recommended for Go users)

If you have Go installed, you can install directly from the repository:

```sh
go install github.com/laszukdawid/terminal-agent/cmd/agent@latest
```

This will install the `agent` binary in your `$GOPATH/bin` directory (typically `~/go/bin`). Make sure this directory is in your PATH.

### Option 3: Compile from source

You can compile the code yourself.
To do so, make sure you have golang installed for compilation and we recommend using [Taskfile](https://taskfile.dev/installation/) for task execution.
If you only want to compile the repo then execute `task build`.
In case you want to both build and set it in your path then execute `task install` which will put binary into `~/.local/bin/agent`.

For majority of cases, do check out the `Taskfile.dist.yaml` as it has most relevant tasks / receipies.

## Usage

**Note**: Default model is OpenAI `gpt-4o-mini`. See [Config](#config) below on how to change it.

### Ask

Intended for general questions.

```sh
agent ask what is a file descriptor?
```

If you like to see characters appearing in terminal, add `--stream` flag. By default, all results are formatted as a markdown using [glamour](https://github.com/charmbracelet/glamour).

**Life hack**: If you set alias `alias aa="agent ask"` you'll have a quick shortcut to ask questions from terminal. It's quicker than opening browser to search! Execute `task install:alias` for auto-setup.

Read more at [Ask Command documentation](https://laszukdawid.github.io/terminal-agent/commands/ask.html).

### Task

Generates code for execution, and asks for execution permissions.

```sh
agent task list files 3 dirs up
```

**Note**: Since more testing is required, there are only a few methods allowed.

### MCP

Terminal Agent supports tools via MCP. To enable them, provide a file path to the JSON file, [mcp.json](/docs/examples/mcp.json). In case you have the file written at `/home/user/mcp.json` you'd update the terminal agent with

```shell
agent config set mcp-path /home/user/mcp.json
```

then to verify it's fine execute

```shell
agent config get mcp-path
```

### Config

Given that a few providers and their models are supported, there's a rudamentry configuration support. Whole config 

Check all settings with `agent config get all`.

## Providers supported

To change the provider (to `$PROVIDER`) and model (to `$MODEL`) do

```sh
$ agent config set provider $PROVIDER
$ agent config set model $MODEL
```

More details at [Providers documentation](https://laszukdawid.github.io/terminal-agent/providers.html).

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

Perplexity AI has an "in development" functionality to provide access to models via their API. To read more on this visit their [documentation](https://docs.perplexity.ai/guides/getting-started).

To use `agent` with Perplexity AI, you need to set `PERPLEXITY_KEY` env variable with your individual key, and then set the provider to `perplexity`.

Setting `perplexity` provider with `llama-3.1-8b-instruct` model can be done with `task run:set:perplexity` or directly

```sh
$ agent config set provider perplexity
$ agent config set model llama-3.1-8b-instruct
```

### OpenAI

To use `agent` with OpenAI, you need to set the `OPENAI_API_KEY` environment variable with your individual key, and then set the provider to `openai`.

Setting `openai` provider with `gpt-4o-mini` model can be done with `task run:set:openai` or directly

```sh
$ agent config set provider openai
$ agent config set model gpt-4o-mini
```

### Ollama

Ollama allows you to run large language models locally on your machine. This provides privacy, offline capability, and no API costs. To use Ollama with the terminal agent, you first need to install and run Ollama on your system.

To get started with Ollama:
1. Install Ollama from [ollama.ai](https://ollama.ai)
2. Pull a model, e.g., `ollama pull llama3.2`
3. Make sure Ollama is running (usually on `http://localhost:11434`)

By default, the terminal agent will connect to Ollama at `http://localhost:11434`. If you're running Ollama on a different host or port, you can set the `OLLAMA_HOST` environment variable:

```sh
export OLLAMA_HOST=http://your-ollama-host:11434
```

Setting `ollama` provider with `llama3.2` model can be done directly:

```sh
$ agent config set provider ollama
$ agent config set model llama3.2
```

**Note**: Make sure the model you specify is already pulled and available in your local Ollama installation. You can list available models with `ollama list`.



## Features

- [x] **ask**: Ask general questions via terminal (all models)
- [x] **task**: Ability to propose and execute a command (expect Perplexity)
- [x] **stream**: Ask only (except Perplexity)
- [x] **markdown**: By default, provide nicely formatted outputs in terminal
- [x] **MCP**: Supports Model Context Protocol (MCP) defined in a file
- [x] **websearch**: Can search the web and display links
- [x] **unix**: Designs and evaluates a unix command

In case some capabilities are missing please create a Feature Request github issue. Looking forward to extending this agent with useful features.

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

More details at [Configuration documentation](https://laszukdawid.github.io/terminal-agent/configuration.html).

## Development

### Setup

* Taskfile, instead of Makefile. See [Taskfile installation](https://taskfile.dev/installation/)
* Docker, needed for running integ tests
* Go 1.23.x or higher
* See [Developer documentation](https://laszukdawid.github.io/terminal-agent/developers.html) for complete setup instructions

### Running Tests

The project uses `task` for test execution:

```sh
# Run all tests
task test

# Run unit tests only
task test:unit

# Run integration tests only
task test:integration
```

### Versioning Strategy

This project follows [Semantic Versioning](https://semver.org/) (SemVer) and uses automated version tagging on the main branch.

#### Automatic Version Bumping

When changes are merged to the `main` branch, the CI/CD pipeline automatically:
1. Runs all tests (`task test`)
2. Determines the version bump type based on commit messages
3. Creates a new version tag
4. Publishes a GitHub release

#### Version Bump Rules

The version bump is determined using [Conventional Commits](https://www.conventionalcommits.org/):

- **PATCH** version (x.x.X) - Default for all changes:
  - `fix:` Bug fixes
  - `chore:` Maintenance tasks
  - `docs:` Documentation changes
  - Any commit without a conventional prefix

- **MINOR** version (x.X.0) - New features:
  - `feat:` New feature additions
  - Example: `feat: add support for new LLM provider`

- **MAJOR** version (X.0.0) - Breaking changes:
  - Any commit with `!` suffix: `feat!:`, `fix!:`, `refactor!:`
  - Commits containing `BREAKING CHANGE:` in the body
  - Example: `feat!: redesign configuration API`

#### Manual Version Override

For cases where automatic detection isn't suitable, maintainers can manually trigger a release with a specific version bump type through the GitHub Actions UI:

1. Go to Actions → "Test and Tag Release"
2. Click "Run workflow"
3. Select the version bump type (patch/minor/major)
4. Run the workflow

#### Commit Message Examples

```sh
# Results in PATCH bump (0.3.0 → 0.3.1)
git commit -m "fix: correct parsing error in ask command"
git commit -m "chore: update dependencies"

# Results in MINOR bump (0.3.1 → 0.4.0)
git commit -m "feat: add Ollama provider support"
git commit -m "feat(mcp): implement new MCP protocol tools"

# Results in MAJOR bump (0.4.0 → 1.0.0)
git commit -m "feat!: change configuration file format"
git commit -m "refactor!: remove deprecated API endpoints"
```

### Contributing

Please ensure your commits follow the conventional commit format to help maintain proper versioning. See [CONTRIBUTING.md](CONTRIBUTING.md) for more details.
