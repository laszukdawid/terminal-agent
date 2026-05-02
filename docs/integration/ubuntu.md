# Ubuntu Integration

This guide covers the supported Ubuntu desktop integration path for the Terminal Agent popup GUI.

## What this does

Running `task integration:ubuntu` installs the popup GUI for the current user and configures the desktop launcher path around `~/.local/bin/agent-gui`.

The integration script performs these steps automatically:

- checks that the host is Ubuntu (`ID=ubuntu`)
- installs missing Fyne desktop build dependencies with `apt-get`
- builds `agent-gui`
- installs `agent-gui` to `~/.local/bin/agent-gui`
- installs a desktop entry in `~/.local/share/applications/terminal-agent-gui.desktop`
- installs the popup icon in the user icon directory
- refreshes the desktop application database when available
- validates the desktop entry
- detects GNOME or KDE Plasma and prepares the supported shortcut path for that desktop
- supports `--uninstall` to remove the installed user-scoped integration files and shortcut configuration

## Run the integration

```sh
task integration:ubuntu
```

To uninstall the Ubuntu integration:

```sh
bash scripts/integ_ubuntu.sh --uninstall
```

The script is user-scoped. It installs the launcher and binary in your home directory.

## Resulting behavior

After setup:

- `agent-gui` starts the popup app normally
- `agent-gui --show` reopens the hidden popup in the existing instance
- `agent-gui --new` starts a separate isolated popup instance for local testing
- pressing `Escape` hides the popup
- the desktop shortcut is configured to run `agent-gui --show`
- the popup `Settings` button updates the shared default provider/model used by GUI asks

## Ubuntu GNOME

On GNOME, the integration script configures a custom shortcut for `~/.local/bin/agent-gui --show` using `gsettings`.

Default shortcut:

```text
Ctrl+Shift+Space
```

If that binding conflicts with your current setup, change it in the GNOME keyboard shortcut settings after the script finishes.

## Ubuntu KDE Plasma

On KDE Plasma, the current verified working path is desktop-entry discovery through Plasma Shortcuts.

After running `task integration:ubuntu`, Plasma discovers the installed `Terminal Agent Popup` launcher in the Shortcuts settings. You can then assign a shortcut to that discovered application entry, and Plasma will launch the popup even when `agent-gui` is not already running.

Suggested setup steps:

1. Run `task integration:ubuntu`.
2. Open `System Settings` -> `Keyboard` -> `Shortcuts`.
3. Find `Terminal Agent Popup` in the applications list.
4. Select it and add a shortcut such as `Ctrl+Shift+Space`.
5. Save the shortcut.

Default shortcut:

```text
Ctrl+Shift+Space
```

Fallback if Plasma does not expose the application entry correctly:

1. Open `System Settings` -> `Keyboard` -> `Shortcuts`.
2. Create a custom command shortcut.
3. Set the command to `~/.local/bin/agent-gui --show`.
4. Bind `Ctrl+Shift+Space` or another available shortcut.

## Files installed

```text
~/.local/bin/agent-gui
~/.local/share/applications/terminal-agent-gui.desktop
~/.local/share/icons/hicolor/256x256/apps/terminal-agent.png
```

These files are removed again by `scripts/integ_ubuntu.sh --uninstall`.

## Verification

The script validates the desktop entry and checks that the installed binary is runnable.

You can also verify manually:

```sh
~/.local/bin/agent-gui
~/.local/bin/agent-gui --show
~/.local/bin/agent-gui --new
desktop-file-validate ~/.local/share/applications/terminal-agent-gui.desktop
```

## Troubleshooting

If shortcut automation does not complete:

- confirm whether you are running GNOME or KDE Plasma
- verify that `~/.local/bin` is present and readable
- run `~/.local/bin/agent-gui --show` directly to confirm the popup path works
- on KDE Plasma, first try the discovered `Terminal Agent Popup` application entry in Shortcuts
- if that does not work, fall back to a custom command shortcut for `~/.local/bin/agent-gui --show`
- re-run `task integration:ubuntu` after fixing any missing desktop tooling
