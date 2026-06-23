# macOS Integration

This guide covers the supported macOS desktop integration path for the Terminal Agent Graphical UI.

## Install the released GUI

For normal use, install the released Terminal Agent package instead of building `agent-gui` from the repository:

```sh
brew install terminal-agent
```

If Homebrew cannot find the formula directly, install it from the project tap:

```sh
brew install laszukdawid/tap/terminal-agent
```

This installs `agent-gui` on your `PATH`. Verify it with:

```sh
agent-gui --help
agent-gui --show
```

Use the source-tree integration below only when you want the repo script to build a local `Terminal Agent.app` bundle in `~/Applications/` and create the `~/.local/bin/agent-gui` symlink.

## Source-tree integration

Running `task integration:macos` installs the Graphical UI as a native `.app` bundle in `~/Applications/` and creates a CLI symlink at `~/.local/bin/agent-gui`.

The integration script performs these steps automatically:

- checks that the host is macOS (Darwin)
- checks for Xcode Command Line Tools
- builds `agent-gui`
- converts `assets/icon.png` to `.icns` format using native macOS tools (`sips` and `iconutil`)
- creates a `.app` bundle with proper `Info.plist`
- installs the bundle to `~/Applications/Terminal Agent.app`
- creates a CLI symlink at `~/.local/bin/agent-gui`
- registers the app with Launch Services for Spotlight indexing
- validates the bundle structure and `Info.plist`
- supports `--uninstall` to remove the app bundle and CLI symlink

## Run the source-tree integration

```sh
task integration:macos
```

To uninstall the macOS integration:

```sh
bash scripts/integ_macos.sh --uninstall
```

The script is user-scoped. It installs the app bundle and symlink in your home directory.

## Resulting behavior

After setup:

- `agent-gui` starts the popup app normally
- `agent-gui --show` reopens the hidden popup in the existing instance
- `agent-gui --new` starts a separate isolated popup instance for local testing
- pressing `Escape` hides the popup
- the popup `Settings` button updates the shared default provider/model used by GUI asks
- the first voice recording from the packaged app may trigger a macOS microphone permission prompt
- the app appears in Spotlight and Finder under `~/Applications/`

## Keyboard shortcut

macOS does not provide a scriptable global shortcut API, so shortcut setup is a manual step.

If you installed `agent-gui` with Homebrew, use the absolute path reported by `which agent-gui` in the shortcut command. This is usually `/opt/homebrew/bin/agent-gui --show` on Apple Silicon or `/usr/local/bin/agent-gui --show` on Intel Macs.

If you ran `task integration:macos`, use `~/.local/bin/agent-gui --show`.

Suggested shortcut:

```text
Ctrl+Shift+Space
```

### Option A: Shortcuts.app (macOS 13+)

1. Open Shortcuts.app.
2. Create a new shortcut.
3. Add a **Run Shell Script** action with your `agent-gui --show` path.
4. Name it **Terminal Agent Popup**.
5. Right-click the shortcut (or open its details).
6. Click **Add Keyboard Shortcut** and press `Ctrl+Shift+Space`.

### Option B: Automator Quick Action

1. Open Automator and create a new **Quick Action**.
2. Add **Run Shell Script** with your `agent-gui --show` path.
3. Save as **Terminal Agent Popup**.
4. Open **System Settings** > **Keyboard** > **Keyboard Shortcuts** > **Services**.
5. Find **Terminal Agent Popup** and assign `Ctrl+Shift+Space`.

## Files installed

```text
~/Applications/Terminal Agent.app/
  Contents/
    Info.plist
    MacOS/
      agent-gui
    Resources/
      terminal-agent.icns
~/.local/bin/agent-gui  (symlink -> Terminal Agent.app/Contents/MacOS/agent-gui)
```

These files are removed by `scripts/integ_macos.sh --uninstall`.

## Verification

The script validates the app bundle structure, runs `plutil -lint` on the `Info.plist`, and checks that the installed binary is runnable.

You can also verify manually:

```sh
~/.local/bin/agent-gui
~/.local/bin/agent-gui --show
~/.local/bin/agent-gui --new
plutil -lint ~/Applications/Terminal\ Agent.app/Contents/Info.plist
```

## Troubleshooting

If the build fails:

- confirm Xcode Command Line Tools are installed: `xcode-select -p`
- if missing, install with: `xcode-select --install`

If the app does not launch from Finder or Spotlight:

- check that `~/Applications/Terminal Agent.app` exists and contains the expected bundle structure
- try running `~/.local/bin/agent-gui` directly to confirm the binary works
- if macOS blocks the app with a Gatekeeper warning, clear the quarantine attribute: `xattr -cr ~/Applications/Terminal\ Agent.app`

If `agent-gui` is not found in your shell:

- ensure `~/.local/bin` is in your `PATH`
- add it with: `echo 'export PATH="$PATH:$HOME/.local/bin"' >> ~/.zshrc`
