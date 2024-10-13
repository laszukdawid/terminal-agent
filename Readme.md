# Terminal agent

An LLM Agent to help you from and within the terminal.

## Features

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
