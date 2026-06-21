# Daemon Command

The `daemon` command runs and manages the background scheduler that fires
[routines](routine.md) automatically. Terminal Agent has no cloud backend, so a
local long-lived process owns scheduling: it holds an in-process cron over every
enabled routine that has a schedule, and when one is due it launches
`agent routine run <id> --scheduled` as an isolated subprocess.

Subprocess isolation means a hang, crash, or model failure in one routine cannot
take down the daemon or other runs, and each run goes through the exact same path
as a manual `agent routine run`.

## Usage

```sh
agent daemon <subcommand>
```

| Subcommand | Purpose |
|------------|---------|
| `start`     | Run the scheduler in the foreground. This is what the service manager executes; you can also run it directly to watch it work. |
| `status`    | Report whether the daemon is running, its PID, the number of scheduled routines, and the next fire time. |
| `stop`      | Signal the running daemon to stop. |
| `install`   | Register the daemon with the OS service manager so it starts on login and restarts on failure. |
| `uninstall` | Remove the daemon from the OS service manager. |

## Running automatically

`install` registers the **daemon itself** as a per-machine OS service (one entry,
not one per routine):

- **macOS** — a launchd LaunchAgent at
  `~/Library/LaunchAgents/com.terminal-agent.daemon.plist` (`RunAtLoad`, `KeepAlive`),
  loaded with `launchctl`.
- **Linux** — a systemd user unit at `~/.config/systemd/user/terminal-agent.service`
  (`Restart=on-failure`, `WantedBy=default.target`), enabled with `systemctl --user`.

```sh
agent daemon install     # start now and on every login
agent daemon status
agent daemon uninstall   # stop and remove the service entry
```

On unsupported platforms, run `agent daemon start` under your own supervisor.

## Behavior

- Only **enabled** routines with a non-empty `--cron` schedule are scheduled; the
  global `routines.enabled` toggle disables all scheduling at once.
- The daemon watches the routines definitions file and reloads on change, so
  `create` / `enable` / `disable` / `delete` take effect without a restart.
- It publishes each routine's next run time, which `agent routine list` / `show`
  display.
- A routine whose previous run is still in progress is skipped (single-flight), not
  run concurrently.
- Only one daemon runs at a time (single-instance lock).
- Routine default changes (model, budgets) take effect on the next run with no
  daemon restart, because each run reloads configuration in its own process.

## Limitations

- The host must be on and the daemon running at the scheduled time. Runs missed
  while the daemon was down are **not** caught up — the routine simply fires on its
  next schedule.
- Schedules use standard 5-field cron expressions (and `@hourly`, `@daily`,
  `@every 1h`, etc.).
