# Auth Command

The `auth` command manages provider credentials. It stores API keys and OAuth tokens separately from the main config file, in `~/.config/terminal-agent/auth.json`.

Currently, only the `openai` provider is supported.

## Usage

```sh
agent auth <subcommand> [args]
```

## Subcommands

### login

Authenticate with a provider:

```sh
agent auth login openai
agent auth login openai --device
agent auth login openai --api-key
```

- `agent auth login openai` starts the browser OAuth flow. Opens the system browser by default, then listens on a local callback server to receive the authorization code.
- `agent auth login openai --device` starts the terminal-friendly device-code flow. Prints a URL and a one-time code to paste in your browser. Polls for authorization for up to 15 minutes (5 second default interval).
- `agent auth login openai --api-key` stores an API key without using OAuth.

Browser login supports a pasted-code fallback. If the localhost callback does not complete automatically, paste either:

- the full redirect URL
- the raw authorization code
- `code#state`
- a query string containing `code=` and optional `state=`

When `--api-key` is used, the command reads the key from the `--key` flag, the `OPENAI_API_KEY` environment variable, an interactive terminal prompt, or stdin (in that order). The terminal prompt hides keystrokes with a password-style masked input.

```sh
# Store an API key from the environment
agent auth login openai --api-key

# Store a specific key
agent auth login openai --api-key --key sk-proj-...

# Read the key from a pipe
echo "$OPENAI_API_KEY" | agent auth login openai --api-key
```

### status

Show auth status for a provider:

```sh
agent auth status openai
```

Example output:

```text
Provider: openai
Configured: yes
Path: /home/user/.config/terminal-agent/auth.json
Auth type: api_key
Source: stored
```

If `OPENAI_API_KEY` is set in the environment, `Source` will show `environment` instead.

For OAuth logins, status also shows `Account ID`, `Plan type`, `Expires`, and `Expired` when that metadata is available.

### logout

Remove stored credentials for a provider:

```sh
agent auth logout openai
```

This removes the entry from `~/.config/terminal-agent/auth.json`. It does not unset environment variables.

## Credential Storage

Credentials are stored in:

```
~/.config/terminal-agent/auth.json
```

The file is created with `0600` permissions and uses atomic writes (write to temp file, sync, rename) to avoid data corruption. File operations are protected by advisory file locks to prevent corruption from concurrent `agent auth` processes.

For stored OAuth logins, Terminal Agent refreshes access tokens automatically while a valid refresh token is still present. If a refresh fails (e.g., the refresh token expired), you will need to run `agent auth login openai` again.

## Auth Resolution

When the agent needs an OpenAI credential at runtime, it resolves in this order:

1. `OPENAI_API_KEY` environment variable — highest priority
2. Stored API key in `auth.json`
3. Stored OAuth credential in `auth.json`
4. Error: authentication not configured

This means you can have stored credentials as a default and temporarily override them by setting `OPENAI_API_KEY`.
