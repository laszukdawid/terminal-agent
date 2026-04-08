# Plugin Command

The `plugin` command installs and manages Terminal Agent plugins.

## Usage

```sh
agent plugin <subcommand> [args]
```

## Subcommands

### install

Install a plugin by name:

```sh
agent plugin install <plugin-name>
```

### uninstall

Uninstall a plugin by name:

```sh
agent plugin uninstall <plugin-name>
```

Optional cleanup:

```sh
agent plugin uninstall bash-reader --purge-data
```

## Available Plugins

### bash-reader

Installs shell integration for terminal context capture.

```sh
agent plugin install bash-reader
```

Use with ask command:

```sh
agent ask "why the command failed" --use-terminal-context 3
# or shortcuts
agent ask "why the command failed" -3
```

What this install does:

- Writes plugin script to `$HOME/.config/terminal-agent/plugins/bash-reader/init.bash`
- Appends a managed source block to `$HOME/.bashrc`
- Creates terminal context storage at `$HOME/.local/share/terminal-agent/terminal-context/`

What this plugin captures:

- Latest commands and exit codes (safe mode)
- A command index entry per shell prompt

Where data is stored:

- Command index: `$HOME/.local/share/terminal-agent/terminal-context/index.log`
- Session directory: `$HOME/.local/share/terminal-agent/terminal-context/sessions/`

`index.log` format (tab-separated):

1. Unix timestamp
2. Exit code
3. Output path (currently empty in safe mode)
4. Base64-encoded command text

Note: The current safe-mode hook avoids global stdout/stderr redirection to keep terminal colors and interactive editors working correctly.

After install, restart your shell or run:

```sh
source ~/.bashrc
```

To uninstall:

```sh
agent plugin uninstall bash-reader
```

Uninstall behavior:

- Removes only the managed `bash-reader` block from `$HOME/.bashrc`
- Keeps other terminal-agent blocks intact (for example aliases)
- Removes plugin files from `$HOME/.config/terminal-agent/plugins/bash-reader/`
- Keeps terminal context data by default
- Use `--purge-data` to also delete `$HOME/.local/share/terminal-agent/terminal-context/`
