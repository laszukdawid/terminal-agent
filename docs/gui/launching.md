# Launching & Shortcuts

The window is designed to live in the background and appear on a keystroke. The
desktop integration tasks bind **<kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>Space</kbd>**
to show or hide it:

- Ubuntu: `task integration:ubuntu`
- Fedora: `task integration:fedora`
- macOS: `task integration:macos`

On Linux these install `agent-gui` into `~/.local/bin/agent-gui`, add a desktop
entry, and (on GNOME) bind the shortcut for you; on KDE Plasma you bind
`Terminal Agent Popup` to the shortcut yourself in System Settings. On macOS the
task builds a `Terminal Agent.app` bundle in `~/Applications/` and symlinks the
binary. See the [Integration guides](../integration/macos.md) for the per-platform
details.

To run it directly during development, use `task run:gui`. The window opens at a
default size of 860×600 and is resizable. Closing the window hides it rather than
quitting, so the next shortcut press brings it straight back; where the desktop
supports it, a system tray menu offers **Show**, **Hide**, and **Quit**.

## Input and keyboard shortcuts

The input box is multi-line and word-wrapped. The sidebar on the left switches
between **Ask**, **Task**, **History**, and **Settings**; the model currently in
use is shown in the top-right (`MODEL: provider / model`) with a status dot. Each
mode keeps its own view, so switching tabs does not discard the last run.

| Shortcut | Action |
| --- | --- |
| <kbd>Enter</kbd> | Submit the prompt |
| <kbd>Shift</kbd>+<kbd>Enter</kbd> | Insert a newline |
| <kbd>Esc</kbd> | Hide the window (or close an open detail/dialog) |
| <kbd>Cmd/Ctrl</kbd>+<kbd>L</kbd> | Focus the input box |
| <kbd>Cmd/Ctrl</kbd>+<kbd>Q</kbd> | Quit the app |
| <kbd>Ctrl</kbd>+<kbd>Shift</kbd>+<kbd>Space</kbd> | Show/hide the window (set by the integration tasks) |
