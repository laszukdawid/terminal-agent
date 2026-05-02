# Popup GUI Plan

Date: 2026-04-27
Status: In Progress
Scope: Add a durable, cross-platform popup GUI for Terminal Agent with strong support for Linux and macOS, using Fyne and a shared application service layer.

## Status Update

Current repository state as of 2026-05-01:

- `internal/app` now exists and is wired into the CLI for `ask`, `chat`, and `task`
- prompt resolution, memory prompt inclusion, request assembly, and connector/tool-provider setup are now shared below Cobra command handlers
- terminal context and related shared path logic were moved out of command-only code into `internal/app`
- CLI presentation behavior remains in command handlers, preserving existing behavior
- `AskEvents` and `ChatEvents` now exist and are wired through the shared app service
- connector history/streaming support was updated enough for GUI-driven ask/chat flows
- `cmd/agent-gui` and `internal/gui` now exist with an initial Fyne popup implementation
- the GUI can launch locally, submit an `ask`, stream output, cancel, copy, and display provider/model
- `internal/platform` now exists with single-instance locking and local IPC activation support
- repeated `agent-gui --show` invocations now reactivate the existing popup instead of opening duplicates
- `Escape` and window close now hide the popup, and the hidden instance can be shown again via IPC
- Fedora desktop integration assets, docs, and installer/uninstaller automation now exist
- the GUI now installs a system tray/status menu with show, hide, and quit actions
- system tray/status behavior is now working in local testing on Fedora KDE
- tests cover the new app-layer helpers and the full Go test suite currently passes

Not yet done:

- the popup UX is still rough and does not yet meet the intended compact quick-access design
- desktop integration artifacts and Fedora docs now exist, but broader verification and cleanup are still pending

Still incomplete within the current popup lifecycle work:

- `agent-gui --show` now behaves as "show if already running, otherwise start visible" so desktop launchers and shortcuts have a reliable first-launch path
- single-instance and IPC support currently target Unix-style local sockets for Linux/macOS; no Windows path exists
- hide/show behavior now works for the current popup flow, but broader popup polish and keyboard/layout refinement are still pending
- Fedora KDE now has one verified working path: install integration, let Plasma discover `Terminal Agent Popup`, and bind the shortcut in System Settings
- KDE shortcut automation is still unresolved; the currently supported path is manual shortcut binding against the discovered application entry, with command-shortcut fallback if discovery fails
- system tray/status presence is now validated on Fedora KDE, but still needs validation on other supported environments

This means the shared backend, first popup implementation, and the core durable popup lifecycle and IPC activation path are now in place. Desktop integration and final popup polish milestones are still pending.

## 1. Goal

Build a small, well-architected desktop GUI companion for Terminal Agent that:

- opens from a global shortcut or desktop-bound shortcut
- shows a compact popup input window
- lets the user ask a question quickly
- displays the answer in the popup
- reuses the existing Go backend logic
- is designed to last, not as a one-off wrapper around the CLI

This first version is intentionally narrow. It is not a full desktop product. It is a focused popup interface for `ask`-style interactions.

---

## 2. Product Intent

The GUI should feel like a quick-access assistant:

- available on demand
- lightweight
- fast to open
- keyboard friendly
- minimally distracting
- architecturally aligned with the CLI

It should not duplicate the CLI implementation in UI code. Both CLI and GUI should depend on the same application layer.

---

## 3. Non-Goals for v1

The following are explicitly out of scope for the first implementation unless they fall out naturally from the architecture:

- full replacement for the CLI
- broad task/tool execution workflows in the GUI
- rich multi-window workspace UI
- advanced chat session management UI
- native global hotkey support on every Linux desktop environment
- tray-heavy desktop behavior unless needed for single-instance control
- custom design system or polished desktop aesthetics beyond clean usability

These can come later. The first release should be useful, stable, and maintainable.

---

## 4. User Experience Target

### Core flow

1. User triggers a shortcut.
2. Popup window appears centered and focused.
3. User types a question.
4. User presses Enter or clicks submit.
5. Response begins streaming into the popup.
6. User can copy the answer.
7. User can press Escape to hide the popup.

### Expected behavior

- startup should be reasonably fast
- popup should remember window size and optionally last position later
- input should autofocus every time popup opens
- response area should support selection and copy
- user should be able to cancel an in-flight request
- errors should be visible and plain

### Keyboard behavior

- `Enter`: submit
- `Shift+Enter`: newline if multiline input is supported
- `Escape`: hide popup
- `Ctrl/Cmd+L`: focus input again later if desired
- `Ctrl/Cmd+C`: copy selected output

---

## 5. Platform Strategy

### Linux

Linux support is a priority, but global hotkeys are fragmented across desktop environments, especially under Wayland.

For a lasting design:

- the app must support being opened via desktop-environment-configured shortcuts
- native in-app global shortcut support should be treated as optional and platform-dependent
- the architecture must not depend on universal low-level keyboard grabbing

### macOS

macOS support matters because the CLI is used there often.

The GUI architecture should remain cross-platform:

- Fyne for UI
- platform adapters for shortcuts and activation
- shared backend logic in Go

---

## 6. Technology Choice

### UI Toolkit: Fyne

Rationale:

- Go-native
- cross-platform enough for Linux and macOS
- avoids adding a JS/web frontend stack
- suitable for a compact utility window
- simpler long-term maintenance for this repository

Tradeoffs accepted:

- UI polish will be pragmatic rather than premium
- some desktop-native behavior may require adaptation or custom handling
- global shortcut support is not solved by Fyne itself

---

## 7. Architectural Principles

The implementation should follow these rules:

1. **No shelling out to `agent ask` from the GUI**
   - GUI must call shared Go code directly.

2. **CLI and GUI share the same application service layer**
   - business logic should not live in Cobra commands.

3. **UI stays thin**
   - Fyne code should focus on rendering and user interaction.

4. **Requests should be event-based**
   - responses should stream through structured events, not just a final string.

5. **Single-instance behavior should be explicit**
   - popup activation should work by signaling a running app instance.

6. **Platform-specific functionality should be isolated**
   - shortcut behavior and app activation should be behind interfaces.

7. **Configuration should remain shared**
   - GUI should use the same config and defaults as the CLI where practical.

---

## 8. High-Level Architecture

### Layers

#### A. Domain/integration layer (existing, reused)
Existing packages already provide most core behavior:

- `internal/agent`
- `internal/connector`
- `internal/config`
- `internal/chat`
- `internal/history`
- `internal/memory`
- `internal/tools`

These should remain below the application layer.

#### B. Application layer
The repository now has an `internal/app` package that exposes use cases in a UI- and CLI-friendly way.

Current status:

- implemented in the repository
- currently provides shared runtime/setup plus `Ask`, `Chat`, and `Task` use cases
- currently provides result-returning APIs plus `AskEvents` and `ChatEvents`

Current responsibilities:

- build requests from config and inputs
- resolve prompts
- invoke the underlying agent/connector logic
- expose event streams
- manage cancellation
- expose configuration and session operations through stable APIs

#### C. GUI layer (new)
Planned package: `internal/gui`, responsible for:

- popup window creation
- input/output widgets
- state binding
- invoking `internal/app`
- showing progress, errors, and results
- managing hide/show behavior

#### D. Platform integration layer (new)
Planned package for platform-dependent behavior:

- single-instance lock
- local IPC
- activation/show-popup signaling
- global shortcut support later

---

## 9. Package Layout

Current and planned layout:

```text
cmd/
  agent/
  agent-gui/

internal/
  app/
    service.go
    runtime.go
    ask.go
    chat.go
    task.go
    prompt.go
    context.go
    paths.go
    terminal_context.go
  gui/
    app.go
    popup_window.go
    presenter.go
    state.go
    actions.go
  platform/
    ipc.go
    single_instance.go
    activation.go
    shortcuts.go
```

### Package responsibilities

#### `cmd/agent-gui`
- GUI entrypoint
- initializes logging, config, application services, and Fyne app
- starts IPC server if primary instance
- sends activation command if secondary invocation

#### `internal/app`
- stable service boundary for both CLI and GUI
- encapsulates agent setup and prompt resolution
- currently exposes result-based use cases for `ask`, `chat`, and `task`
- later needs a request/response event API for GUI streaming

#### `internal/gui`
- popup window and state management
- no provider or connector-specific logic
- updates UI from application events

#### `internal/platform`
- single-instance behavior
- local IPC
- future native shortcut registration adapters

---

## 10. Application Service API

The application service exposes durable use cases today and should remain the shared service boundary going forward.

Current repository status:

- a shared `Service` interface already exists in `internal/app`
- it currently exposes `Ask`, `Chat`, and `Task`
- `Ask` and `Chat` now also have event-streaming entrypoints
- event work should preserve separate service entrypoints for now rather than forcing a generic `Run(...)` API too early

### Current API

```go
type Service interface {
    Ask(ctx context.Context, req AskRequest) (AskResult, error)
    Chat(ctx context.Context, req ChatRequest) (ChatResult, error)
    Task(ctx context.Context, req TaskRequest) (TaskResult, error)
}
```

### `AskRequest`

Current fields include:

- `Message string`
- `Provider string`
- `Model string`
- `PromptOverride string`
- `UseMemory bool`
- `WorkingDir string`
- `Stream bool`
- `ContextFiles []string`
- `TerminalContextCount int`

### Event model

Current status:

- not implemented yet
- still blocked on a reusable streaming contract below `internal/app`
- tracked separately as follow-up work so Milestone 1 refactoring could proceed without changing existing connector behavior
- first implementation should serve `Ask` and `Chat` immediately while remaining compatible with future `Task` events
- `Task` should not force a second, unrelated event path later

Use typed events instead of raw text return values.

Recommended shape:

- keep `Ask`, `Chat`, and `Task` result-returning APIs for CLI compatibility
- add parallel event APIs as separate methods for now:
  - `AskEvents(...)`
  - `ChatEvents(...)`
  - `TaskEvents(...)` later
- use one shared event envelope across modes so GUI code only needs one subscription/rendering path

Possible shared event types:

- `started`
- `output_delta`
- `status`
- `tool_call_requested`
- `tool_call_completed`
- `confirmation_needed`
- `completed`
- `failed`

Expected initial usage:

- `AskEvents` and `ChatEvents` emit `started`, `output_delta`, `completed`, and `failed`
- future `TaskEvents` reuses the same stream and adds `status`, tool lifecycle, and confirmation events

This intentionally avoids designing the first event layer as ask-only even though ask is the first GUI use case.

### Why this matters

If the app only returns one final string, the design will become brittle when streaming, cancellation, and richer workflows arrive. Event-based output keeps the architecture clean.

---

## 11. Popup Window Design

### Popup contents

For v1, the popup should contain:

- title or subtle app label
- provider/model indicator (optional but useful)
- single input area
- submit button
- cancel button during request
- output/response display area
- copy response button
- lightweight status or error text

### Layout goals

- compact by default
- readable answer area
- comfortable enough for multi-line responses
- straightforward keyboard control

### Window behavior

- starts hidden or opens on demand
- opens focused
- can be hidden instead of destroyed
- one active request at a time in v1
- can be re-opened quickly

### UX constraints

This should feel like a popup assistant, not a full dashboard.

---

## 12. Single-Instance Strategy

A serious popup utility should not open multiple competing windows by default.

### Recommended behavior

- primary instance starts the app and owns the popup window
- subsequent invocations signal the primary instance to show/focus the popup
- secondary instance exits immediately after signaling

### Implementation approach

Use a local IPC channel, for example one of:

- Unix domain socket on Linux/macOS
- lock file plus socket
- simple local command protocol

### Commands supported by IPC initially

- `show`
- `hide`
- `toggle` (optional)
- later: `ask <message>` or `focus`

### Benefits

- clean desktop shortcut integration
- simple scripting integration
- consistent app state
- no duplicate background processes

---

## 13. Shortcut Strategy

### v1 shortcut support

For v1, do not promise universal native global hotkey support across Linux.

Instead:

- support launching/showing the popup with `agent-gui --show`
- let users bind a desktop environment shortcut to that command
- document setup for GNOME and KDE

This is not a compromise in architecture. It is the most honest cross-desktop baseline.

### v2 shortcut support

Add platform adapters where feasible.

Potential abstraction:

```go
type ShortcutManager interface {
    Supported() bool
    RegisterTogglePopup(accelerator string) error
}
```

### Notes

- Linux Wayland support may remain partial or desktop-specific
- Linux X11 support may be possible
- macOS may have a cleaner native path later

---

## 14. Configuration Strategy

The GUI should not invent a separate config system unless absolutely necessary.

### Requirements

- reuse `internal/config`
- read existing user config
- respect provider/model defaults
- support memory-related flags/settings where sensible
- keep CLI and GUI behavior aligned

### GUI config operations for v1

- display current provider
- display current model
- maybe allow changing provider/model in a simple settings screen

### Future config extensions

- default popup size
- startup hidden/start on login
- preferred shortcut label only for docs display
- window persistence settings

---

## 15. Logging and Error Handling

### Logging

The GUI should log meaningful operational events:

- primary/secondary instance detection
- IPC startup/failure
- popup show/hide
- ask request lifecycle
- connector errors

Logs should be file-based and useful for debugging without cluttering the user interface.

### User-facing errors

Errors shown in the popup should be:

- short
- specific
- actionable when possible

Examples:

- missing API key
- invalid provider/model
- network timeout
- prompt resolution failure

---

## 16. Testing Strategy

This should not be built as a UI-only manual test toy.

### Unit tests

Add tests for:

- `internal/app` request construction
- event generation and completion paths
- config integration
- single-instance and IPC protocol parsing where practical

Current status:

- `internal/app` prompt, context, and terminal-context helpers now have direct tests
- CLI command tests continue to cover flag behavior and command integration
- event-generation tests are still pending because the event model is not implemented yet

### Integration tests

Add tests for:

- asking through the app service with mocked connectors where possible
- startup behavior for primary vs secondary invocation

### Manual QA matrix

At minimum:

- Ubuntu GNOME, Wayland
- Fedora GNOME, Wayland
- Fedora KDE if possible
- macOS

### Manual test scenarios

- open popup from desktop shortcut
- submit question
- stream answer
- cancel request
- hide and reopen popup
- missing credentials
- repeated invocations route to same instance

---

## 17. Packaging and Distribution

Packaging should be considered early, even if not fully implemented in the first merge.

### Linux

Target eventually:

- desktop entry
- icon assets
- AppImage and/or Flatpak
- possible native packaging later

### macOS

Target eventually:

- app bundle
- proper app metadata
- signed/notarized distribution later if needed

### Build system impact

Current release flow is CLI-oriented. GUI artifacts will require additional release work, likely including:

- additional build targets
- GUI-specific packaging steps
- possibly updated release automation beyond the current `.goreleaser.yaml`

---

## 18. Migration and Refactoring Plan

The GUI should drive a small but deliberate backend refactor.

### Refactor targets

1. Extract reusable application logic from Cobra command handlers.
2. Create a stable `internal/app` API.
3. Make streaming behavior available outside terminal-print flows.
4. Keep CLI behavior intact by having CLI call the new application layer where practical.

### Important rule

Do not copy logic from CLI commands into GUI code. That creates duplication immediately.

---

## 19. Milestone Plan

### Milestone 1: Application layer extraction

Goal: expose reusable application services without GUI while preserving current CLI behavior.

Deliverables:

- `internal/app` package
- shared runtime/setup for provider, prompts, config, and tool provider creation
- `Ask()`, `Chat()`, and `Task()` service entrypoints used by the CLI
- shared request/config resolution logic
- tests for the new service layer helpers
- follow-up issue for GUI-friendly streaming/event support

Success criteria:

- application services can run ask/chat/task flows without embedding business logic in Cobra command handlers
- CLI reuses the shared application layer while preserving current behavior
- no shell-out-based GUI path is required for future work

Current status:

- complete for the current popup scope
- `AskEvents` and `ChatEvents` are implemented
- `TaskEvents` remains intentionally deferred

### Milestone 2: Basic Fyne popup

Goal: functional popup window driven by the app service.

Deliverables:

- `cmd/agent-gui`
- popup window with input/output controls
- submit, cancel, copy actions
- event-driven response rendering

Success criteria:

- developer can run GUI locally and ask a question successfully
- UI remains responsive during streaming

Current status:

- partially complete
- the popup launches and can submit/stream/cancel/copy
- provider/model is visible in the window
- the popup can currently hide, but there is no user-facing reactivation path yet
- this means popup hide/show is not complete in product terms until the next milestone adds activation support

### Milestone 3: Single-instance + IPC activation

Goal: make popup practical for repeated use.

Deliverables:

- single-instance detection
- IPC server/client support
- `agent-gui --show` behavior
- hidden-to-visible activation path
- a reliable way to bring back a popup hidden with `Escape`

Success criteria:

- second invocation activates the existing window instead of opening duplicates
- a hidden popup can be shown again on demand without restarting the process
- `Escape` hide is only considered complete once this reactivation path exists

Current status:

- largely complete for Linux/macOS popup development
- single-instance locking and local IPC are implemented under `internal/platform`
- `agent-gui --show` now signals the running instance to show the popup again
- `Escape` and window close now hide the popup instead of quitting, and the hidden instance remains available for reactivation
- `agent-gui --show` now doubles as both the reactivation path and a reliable first-launch-visible entrypoint for desktop integration
- desktop shortcut docs/integration are still pending in the next milestone

### Milestone 4: Desktop integration

Goal: make the feature usable on Linux and macOS in practice.

Deliverables:

- `.desktop` entry or equivalent launch support
- docs for GNOME/KDE shortcut setup
- app icon and metadata

Success criteria:

- user can bind a system shortcut to `agent-gui --show`
- popup opens reliably from desktop shortcut

Current status:

- partially complete
- Fedora now has a user-scoped integration script, `.desktop` launcher asset, icon asset, and docs under `docs/integration/`
- the installer places `agent-gui` in `~/.local/bin/agent-gui` and installs desktop entry metadata successfully
- Fedora KDE has now verified the tray/status icon presence and tray menu behavior for show/hide/quit
- GNOME shortcut automation exists; KDE shortcut automation is still not complete in product terms
- Fedora Plasma Wayland now has one verified end-to-end supported path via the discovered `Terminal Agent Popup` application entry in Shortcuts
- the old KDE `_launch` shortcut automation attempt should remain deprecated unless a reliable Plasma automation path is implemented later
- the remaining Milestone 4 work is documenting any GNOME/KDE caveats clearly and validating at least one more supported desktop path

### Milestone 5: Popup behavior and layout polish

Goal: finish the remaining popup-specific behavior after desktop integration proves the launch path.

Deliverables:

- reliable hide/show lifecycle in the GUI app
- input keyboard behavior aligned with popup expectations
- cleaner popup sizing and max-height behavior
- improved single-window question/answer layout
- better default visual hierarchy for the compact popup

Success criteria:

- popup hide/show behavior is dependable and ready to be driven by future IPC activation
- the window behaves like a compact quick-access assistant rather than a generic stacked form

Additional follow-up now identified during desktop integration work:

- validate tray/status behavior across supported Linux desktop environments and macOS
- refine tray behavior if desktop-specific limitations appear during manual QA
- keep the tray/status actions aligned with the single-instance popup lifecycle as KDE/GNOME shortcut work evolves

### Milestone 6: Settings and polish

Goal: reduce friction for daily use.

Deliverables:

- basic settings UI for provider/model
- better error UX
- startup and window polish
- docs updates

Success criteria:

- basic config can be inspected and adjusted in GUI
- app feels coherent, not experimental

---

## 20. Risks and Mitigations

### Risk: Linux global hotkey inconsistency

Impact:
- native shortcut support may not work uniformly

Mitigation:
- rely on desktop-bound shortcut invocation as the guaranteed path
- abstract native shortcut support for future enhancement

### Risk: UI thread misuse during streaming

Impact:
- freezes or unstable behavior

Mitigation:
- channel-based event delivery
- explicit UI-thread marshaling for widget updates
- request cancellation support

### Risk: application logic duplication

Impact:
- CLI and GUI diverge over time

Mitigation:
- introduce `internal/app` early
- move shared logic into it before GUI complexity grows

### Risk: release complexity

Impact:
- GUI artifacts become awkward to build and ship

Mitigation:
- define packaging expectations early
- keep GUI build targets explicit

---

## 21. Deliverables Summary

Planned concrete outputs:

- new `cmd/agent-gui`
- new `internal/app` application service layer
- new `internal/gui` Fyne UI layer
- new `internal/platform` IPC/single-instance support
- popup GUI for ask workflow
- desktop shortcut integration via `--show`
- documentation for Linux and macOS usage

---

## 22. Recommended First Implementation Slice

The first implementation slice should include only what is needed to prove the architecture cleanly:

- app service with shared runtime/setup and ask/chat/task orchestration
- event-based ask streaming as the remaining backend prerequisite for GUI work
- Fyne popup window
- submit/cancel/copy behavior
- reliable hide/show behavior in the current GUI

Do not add task execution, chat history browsers, or complex settings before this slice works well.

---

## 23. Definition of Done for v1

This feature can be considered successfully delivered when:

- a user can install/run `agent-gui`
- one app instance stays available for popup use
- a desktop shortcut can trigger popup visibility
- the popup accepts a question and displays the answer
- the backend path is shared code, not CLI shell-out logic
- the UI remains responsive during requests
- errors are understandable
- the code structure is suitable for future growth

---

## 24. Final Recommendation

Proceed with:

- **Fyne for the GUI**
- **`internal/app` as a shared service boundary**
- **single-instance app + local IPC**
- **desktop-managed shortcut invocation as the guaranteed Linux path**
- **native shortcut adapters later, behind interfaces**

That is the durable design. It is not the fastest possible route, but it is the right one for a long-lived feature in this repository.
