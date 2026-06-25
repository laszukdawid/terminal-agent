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

Routine defaults are set inline in this dialog, in a **Routines** section: the
provider, model, timeout, token budget, max turns, and max tool calls applied to
routines that leave those fields blank. Each field shows its effective default as
a hint and has an info icon that explains the setting on click. The global
routines on/off switch is not here; it lives in the **Routine** tab (see
[Routines](./routines.md)).

See [Providers](../providers.md) for what each provider needs.
