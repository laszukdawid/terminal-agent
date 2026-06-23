# Voice Input

The mic control in the lower-left lets you dictate a prompt instead of typing.
With the window focused, press the configured trigger key or click **Listen** to
start recording; press again (or click **Stop**) to finish. The control reflects
its state (LISTEN → LISTENING → WORKING), transcribes the audio, and drops the
text into the input box. By default voice is enabled, the trigger key is
<kbd>F1</kbd>, and the transcript is submitted automatically through the Ask
path. Recording is toggle-based (not push-to-talk) and requires window focus;
there is no global voice hotkey. Raw audio is kept in memory and never written to
disk.

Voice behavior is configured under the `gui.voice` section of the config (see
[Configuration](../configuration.md)):

| Setting | Default | Meaning |
| --- | --- | --- |
| `gui.voice.enabled` | `true` | Master on/off for voice input |
| `gui.voice.trigger_key` | `F1` | Key that toggles recording |
| `gui.voice.auto_submit` | `true` | Submit the transcript automatically once ready |
| `gui.voice.max_recording_duration` | `60s` | Stop recording after this long |
| `gui.voice.stt.backend` / `gui.voice.stt.model` / `gui.voice.stt.language` | `openai` / `gpt-4o-mini-transcribe` / — | Speech-to-text backend, model, and language |

The default OpenAI backend sends recorded audio to OpenAI for transcription and
reuses your OpenAI auth. Voice is disabled while a response is in flight so it
cannot collide with a running prompt.
