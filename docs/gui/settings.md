# Settings

Open **Settings** to change the provider and model without editing a config
file. The dialog offers a provider field with autocomplete and a model field
that hints the default model for the chosen provider. A status icon (✓ / ✕) next
to the provider shows whether the required credentials or environment are in
place, with a tooltip explaining any problem, and the environment section
surfaces warnings from loading your env file or shell environment. Saving writes
the new defaults to `~/.config/terminal-agent/config.json`, so the choice sticks
across runs and matches what the CLI uses. The dialog footer shows the build
version.

Routine-related defaults and the global routines on/off switch live here too,
under **Settings → Routine defaults…** (see [Routines](./routines.md)).

See [Providers](../providers.md) for what each provider needs.
