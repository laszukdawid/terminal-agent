# Custom Prompt Management Design

## Overview

Add support for customizable system prompts with a priority-based fallback hierarchy. Users can override prompts via CLI flags or project-specific files, enabling per-project agent behavior customization.

## Prompt Resolution Hierarchy

### For `ask` command:
1. `--prompt` flag (highest priority)
2. `./ask/system.prompt` file in working directory
3. `./ask_system.prompt` file in working directory
4. Default `SystemPromptAsk` (lowest priority)

### For `task` command:
1. `--prompt` flag (highest priority)
2. `./task/system.prompt` file in working directory
3. `./task_system.prompt` file in working directory
4. Default `SystemPromptTask` (lowest priority)

All prompt sources support `{{header}}` template substitution for dynamic system context (hostname, username, time, working directory, OS, Go version).

## Implementation Changes

### 1. New function in `internal/agent/prompt.go`

```go
func ResolvePrompt(flagPrompt string, promptType string) string
```

- `promptType` is either `"ask"` or `"task"`
- Checks sources in priority order
- Reads file content if file exists
- Applies `{{header}}` substitution to final result

### 2. File detection logic

```go
// For "ask": check ./ask/system.prompt, then ./ask_system.prompt
// For "task": check ./task/system.prompt, then ./task_system.prompt
```

### 3. CLI changes in `ask_cmd.go` and `task_cmd.go`

- Add `--prompt` string flag to both commands
- Pass flag value to `ResolvePrompt()`

### 4. Update `NewAgent()` in `agent.go`

- Accept resolved prompts instead of using hardcoded defaults
- Keep `{{header}}` substitution in one place

## Error Handling & Edge Cases

### File reading:
- If file exists but can't be read → error out with clear message
- If file is empty → treat as "not present", fall back to next source
- File paths are relative to current working directory

### Template substitution:
- `{{header}}` is optional in custom prompts
- If not present, prompt is used as-is (no error)

### CLI flag:
- `--prompt "string"` → use literal string as prompt
- Could later support `--prompt-file path` if needed (not in this scope)
