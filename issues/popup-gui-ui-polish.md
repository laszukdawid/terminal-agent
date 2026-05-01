# Popup GUI UI Polish

Status: Open
Priority: Medium
Related branch: `feat/popup-gui`

## Problem

The first popup GUI slice launches and streams responses, but the current UI feels like stacked default widgets rather than a compact quick-access popup.

Screenshot review notes:

- the answer panel is too tall and dominates the window
- the controls are detached at the bottom instead of feeling like part of one compact panel
- the question card is visually too heavy
- the layout feels like generic defaults rather than a fast popup assistant
- the popup does not yet feel compact or quick to use

## Follow-up work

1. Compress the vertical layout.
2. Make the input, question, and answer feel like one coherent panel.
3. Improve default spacing and sizing.
4. Move controls and status into a tighter footer.
5. Keep the window resizable while enforcing cleaner max-height behavior.
6. Implement the intended input behavior:
   - `Enter` submits
   - `Shift+Enter` inserts newline
   - input grows up to 3 visible lines, then scrolls

## Not part of the current immediate fix

This ticket is intentionally separate from functional popup issues such as process lifetime, close behavior, IPC activation, and request correctness.
