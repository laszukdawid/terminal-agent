# Developer Guide

This guide provides information for developers who want to contribute to Terminal Agent.

## Development Environment Setup

### Prerequisites

Before you begin, ensure you have the following installed:

1. **Go** - Terminal Agent is written in Go (1.20+ recommended)
   * [Official Go Installation Guide](https://golang.org/doc/install)
   * Verify installation with: `go version`

2. **Taskfile** - Used instead of Makefile for running tasks
   * [Taskfile Installation Guide](https://taskfile.dev/installation/)
   * Verify installation with: `task --version`

3. **Docker** - Required for running integration tests
   * [Docker Installation Guide](https://docs.docker.com/get-docker/)
   * Verify installation with: `docker --version`

### Setting Up the Repository

1. **Clone the repository**
   ```sh
   git clone https://github.com/laszukdawid/terminal-agent.git
   cd terminal-agent
   ```

2. **Install dependencies**
   ```sh
   task setup
   ```

3. **Build the project**
   ```sh
   task build
   ```

4. **Run tests**
   ```sh
   task test
   ```

## Development Workflow

### Common Tasks

Terminal Agent uses Taskfile for managing development tasks. Here are some common commands:

```sh
# Build the project
task build

# Install to your PATH
task install

# Run unit tests
task test

# Run integration tests
task test:integ

# Run the agent with a question
task run:ask -- "What is a file descriptor?"

# Run the agent with a task
task run:task -- "List files in current directory"

# Set environment for different providers
task run:set:openai
task run:set:anthropic
task run:set:bedrock
task run:set:perplexity
task run:set:google
```

To see all available tasks:

```sh
task --list
```

### Project Structure

```
terminal-agent/
├── cmd/                      # Command-line applications
│   └── agent/                # Main agent application
├── internal/                 # Private application and library code
│   ├── agent/                # Agent implementation
│   ├── commands/             # CLI command implementations
│   ├── config/               # Configuration handling
│   ├── connector/            # LLM provider connectors
│   ├── history/              # History logging and retrieval
│   ├── tools/                # Tool implementations
│   └── utils/                # Utility functions
├── docs/                     # Documentation
├── tests/                    # Test files
└── Taskfile.dist.yaml        # Development tasks
```

### Docker Environment

For integration testing and consistent development environments, Terminal Agent uses Docker:

```sh
# Build the test environment
task env:build

# Setup the test environment
task env:setup

# Access the test environment
task env:access

# Run tests in the environment
task env:test
```

## Adding LLM Provider Support

To add support for a new LLM provider:

1. Create a new connector file in `internal/connector/`
2. Implement the `LLMConnector` interface
3. Update the `NewConnector` factory function to include your provider
4. Add appropriate configuration options

## Adding New Tools

To add a new tool:

1. Create a new tool implementation in `internal/tools/`
2. Implement the `Tool` interface
3. Update the `ToolProvider` to return your new tool
4. Add documentation for your tool

## Documentation

Documentation is written in Markdown and stored in the `docs/` directory. To update the documentation:

1. Edit the relevant Markdown files
2. If adding new pages, update the navigation in `docs/Readme.md`

## Building for Release

To build for release:

```sh
task build:release
```

This creates optimized binaries for multiple platforms in the `release/` directory.

## Code Style and Conventions

* Follow standard Go code style and conventions
* Use `go fmt` to format code
* Use `golint` and `golangci-lint` for linting
* Write tests for new functionality
* Document public functions and types

## Pull Request Process

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add/update tests as necessary
5. Ensure all tests pass
6. Update documentation if needed
7. Submit a pull request

## Debugging

For debugging, use the `--loglevel debug` flag:

```sh
go run cmd/agent/main.go --loglevel debug ask "What is a file descriptor?"
```

## Continuous Integration

The project uses GitHub Actions for continuous integration. When you submit a pull request, the CI system will automatically:

1. Build the project
2. Run unit tests
3. Run integration tests
4. Check code formatting

Ensure that all CI checks pass before your pull request can be merged.
