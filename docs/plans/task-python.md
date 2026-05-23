# Task Python Enablement Plan

## Goal

Enable `agent task` to draft Python scripts via native Go tooling (no `cat <<EOF`) and execute them using `python3`, `python`, or `uv run python`.

## Scope

- Add native tools for file editing and searching.
- Add a Python runner tool that supports `python`, `python3`, and `uv run python`.
- Keep Unix tool for general command execution, but expand allowed commands as needed.
- Update prompts so the model prefers native tools for editing and searching.

## Implementation Steps

1. Tools
   - Add `FileEditorTool` for create/update/replace operations with safe path handling.
   - Add `FileSearchTool` for content and filename search with output truncation.
   - Add `PythonRunnerTool` that can:
     - Execute a file with `python3` or `python`.
     - Execute with `uv run python` (including script mode).
     - Capture stdout/stderr and return structured output.

2. Registration
   - Register new tools in `internal/tools/main.go`.
   - Ensure schemas are discoverable via `GetAllBuiltinTools`.

3. Safety and Defaults
   - Reject `sudo` and unsafe paths.
   - Restrict writes to the working directory tree.
   - Provide a clear error when Python/uv is missing.

4. Prompting
   - Update task system prompt to prefer native file editing/search tools.
   - Keep Unix tool for non-edit/search operations.

5. Unix Validation
   - Allow `python`, `python3`, and `uv` in the Unix whitelist to support fallback execution.

## Tests

- Unit tests for each new tool (file edit, file search, python run).
- Unix validation tests for newly allowed commands.
- Integration test covering: create `hello.py`, run with `python3`, and run with `uv run python` when available.

## Acceptance Criteria

- `agent task "Create hello.py that prints Hello and run it"` succeeds without `cat` usage.
- `agent task "Run hello.py with uv run python"` succeeds when `uv` is available.
- Existing Unix tool flows continue to work.
