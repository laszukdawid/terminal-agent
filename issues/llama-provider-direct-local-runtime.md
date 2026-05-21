# Llama Provider Direct Local Runtime

Status: Open
Priority: High

## Problem

The project currently supports local LLM usage through the `ollama` provider, which depends on a separately running REST service. We want a first-party local provider that loads and runs local GGUF-backed llama-family models directly from the CLI and GUI process.

This work should add a new `llama` provider that integrates with the existing connector architecture without introducing prompt hacks or provider-specific behavior leaks into higher layers.

## Goals

Add a direct local `llama` provider with these v1 properties:

- provider name is `llama`
- model selection remains name-based in existing provider config flow
- model names resolve through a config alias map rather than requiring raw file paths
- runtime integration stays behind a small repo-local abstraction so the underlying library can be replaced later
- normal prompt/query flow works for CLI and GUI use cases
- tool calling is supported only when the runtime/model can support a real structured path; otherwise the provider returns a clear error instead of using prompt-only emulation
- model load/unload is process-local for now; per-execution loading is acceptable in v1

## Non-goals For V1

- no background daemon or persistent shared local model service
- no aggressive optimization for repeated invocations yet
- no fake or best-effort prompt-hack tool calling layer
- no broad generic provider capability framework unless implementation pressure proves it necessary
- no automatic remote model download unless it falls out naturally from the chosen runtime and does not complicate the surface area

## Proposed Design

### Provider surface

Add a new connector provider named `llama` and register it anywhere provider lists are validated or defaulted.

Expected initial integration points:

1. `internal/connector/main.go`
2. `internal/config/main.go`
3. `internal/gui/settings_validation.go`
4. user-facing docs/help text where provider options are described

### Runtime backend

Start with `github.com/hybridgroup/yzma` as the direct runtime backend.

Reasons:

- active maintenance
- no CGo requirement
- direct llama.cpp/GGUF integration
- supports streaming
- likely best fit for local direct inference without introducing a local HTTP server dependency

The code should keep yzma contained behind a small internal abstraction so swapping libraries later is feasible without rewriting higher-level connector logic.

### Model selection and resolution

The current provider config flow should continue to treat the model as a logical model name.

V1 resolution strategy:

1. read the configured `providers["llama"]` model name
2. resolve that name via a llama-specific alias map in config
3. optionally allow a direct path only as a fallback if the provided model string already points to an existing file
4. fail with a clear error when the model name cannot be resolved

V1 should default to alias-map-based resolution as the primary mechanism.

### Config shape

The existing `providers map[string]string` should remain the selected provider-model mechanism.

Add llama-specific config for alias resolution. Exact field names can be refined during implementation, but the intended shape is roughly:

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

Requirements:

- alias map keys are stable logical names
- alias map values are local GGUF file paths
- missing alias or unreadable path returns a clear runtime error
- config loading should preserve backwards compatibility with existing config files

### Tool calling policy

`task` support matters, but we do not want to add brittle prompt-based tool emulation.

V1 policy:

- if the runtime path can support structured tool calling reliably, implement `QueryWithTool`
- if the selected model/runtime does not support that path, return a clear error to the user
- do not silently degrade into fake tool calling

### Runtime lifecycle

V1 can load the model in-process for each execution and release it when the process exits.

That means:

- CLI latency may be higher than Ollama for cold starts
- implementation stays simpler
- no singleton daemon or cross-process cache is needed yet

### Prompting and formatting

The connector must map:

- system prompt
- user prompt
- prior messages

into a runtime input format suitable for llama-family local models.

This needs explicit handling rather than assuming all GGUF models behave the same. Implementation should prefer runtime or model-template-aware formatting when available.

## Implementation Outline

1. Add `llama` provider constant and registration.
2. Extend config with llama-specific alias map support.
3. Add a small internal llama runtime wrapper that isolates yzma.
4. Implement model resolution from provider model name to GGUF path.
5. Implement `Query` with streaming support.
6. Implement `QueryWithTool` only if the runtime path is genuinely structured and reliable.
7. Return explicit unsupported errors for tool use when the selected model/runtime cannot support it.
8. Update GUI validation and provider setup hints.
9. Document config requirements and basic local setup.

## Acceptance Criteria

- `llama` is recognized as a supported provider across CLI and GUI validation paths.
- config supports alias-map-based llama model resolution.
- selecting a configured llama model name resolves to a local GGUF file and runs direct local inference.
- streaming query output works for the normal query path.
- unresolved aliases and invalid paths return clear user-facing errors.
- tool-calling behavior is explicit: either supported through a real structured path or rejected with a clear error.
- yzma usage is isolated behind repo-local code rather than leaking through application layers.

## Verification

Manual verification should cover:

1. `ask` with a valid local llama model alias
2. GUI ask flow with the `llama` provider
3. invalid alias error
4. invalid GGUF path error
5. streaming output behavior
6. `task` with a model/runtime that supports tool calling, if available
7. clear `QueryWithTool` error when tool calling is unsupported

## Notes

- v1 intentionally prefers a simple per-process load/unload lifecycle over daemonization or process reuse
- if yzma turns out to block required functionality, the runtime abstraction should make it straightforward to replace without changing the connector contract

## Remaining QueryWithTool Blocker

The current `llama` provider implements direct local `Query()` for ask/GUI flows, but it does not yet implement `QueryWithTool()` for task execution.

This is not blocked by provider wiring, model loading, or basic prompt generation. The remaining problem is the lack of a reliable structured tool-calling path for direct local inference.

### What works already

- provider registration and config integration are complete
- model aliases resolve through `llama_models`
- local GGUF inference works through `yzma` and llama.cpp
- message history and system prompts can be formatted into a model prompt
- streaming output works for normal queries

### What is still missing

`QueryWithTool()` must return structured data in the existing connector contract:

```go
type LlmResponseWithTools struct {
    Response     string
    ToolUse      bool
    ToolName     string
    ToolInput    map[string]any
    ToolResponse any
}
```

Remote providers such as OpenAI, Anthropic, Google, Bedrock, and Ollama expose tool-calling in a provider-native structured form. The connector can reliably read a tool name and parsed arguments from the provider response.

The direct local `llama` path does not currently have that guarantee.

### Why prompt-only parsing is not enough

`yzma` examples demonstrate tool use by instructing the model to emit markup or JSON-like text and then parsing that generated text afterward.

That approach is useful for demos, but it has real failure modes for this codebase:

1. the model may emit invalid JSON
2. the model may emit partial JSON followed by prose
3. the model may emit multiple conflicting tool calls
4. the model may answer directly instead of producing a tool call when the task loop expects one
5. different chat templates or model families may change the output format enough to break parsing

For `agent task`, these are not just formatting bugs. They can cause incorrect tool execution, malformed arguments, or unstable task loops.

### What a robust implementation likely requires

The likely next step is to force a strict structured output format for tool turns.

The response contract should distinguish at least two result types:

1. tool call
2. final answer

An example shape would be:

```json
{
  "type": "tool_call",
  "name": "unix",
  "arguments": {
    "command": "ls -la"
  }
}
```

or:

```json
{
  "type": "final_answer",
  "content": "..."
}
```

To make this reliable, the implementation should prefer grammar-constrained or otherwise strictly constrained generation at sampling time, rather than plain prompt instructions alone.

### Concrete implementation requirements

1. Define a connector-local structured tool output schema.
2. Investigate whether `yzma` grammar samplers can enforce that schema tightly enough.
3. Build a strict parser and validator for the generated output.
4. Return explicit errors when output is invalid instead of attempting permissive recovery.
5. Decide whether tool use should be allowed for all `llama` aliases or only documented/supported models.

### Non-goal for the next implementation

Do not add a brittle prompt-only fallback parser just to make `task` appear supported.

If the local runtime cannot yet guarantee valid structured tool output, `QueryWithTool()` should continue to fail clearly until the constrained path is implemented.
