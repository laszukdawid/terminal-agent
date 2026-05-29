package gui

import (
	xwidget "fyne.io/x/fyne/widget"

	"github.com/laszukdawid/terminal-agent/internal/connector"
)

// providerEntry is an editable autocomplete field for choosing a provider.
// The user may type any value; suggestions are filtered from the connector
// registry by case-insensitive substring, and the full list is shown when the
// field is focused while empty.
type providerEntry struct {
	xwidget.CompletionEntry
}

// newProviderEntry builds the autocomplete field pre-filled with initial.
// onChange (may be nil) is invoked after each text change so callers can update
// dependent UI (e.g. hint labels).
func newProviderEntry(initial string, onChange func(string)) *providerEntry {
	e := &providerEntry{}
	e.ExtendBaseWidget(e)
	e.SetText(initial)
	e.OnChanged = func(s string) {
		opts := filterProviders(connector.SupportedProviders(), s)
		if len(opts) == 0 {
			e.HideCompletion()
		} else {
			e.SetOptions(opts)
			e.ShowCompletion()
		}
		if onChange != nil {
			onChange(s)
		}
	}
	return e
}

// FocusGained shows the full provider list when the field is focused while
// empty. Gaining focus does not fire OnChanged (which only runs on text
// changes), so this does not duplicate the suggestion popup shown while typing.
func (e *providerEntry) FocusGained() {
	e.CompletionEntry.FocusGained()
	if e.Text == "" {
		e.SetOptions(connector.SupportedProviders())
		e.ShowCompletion()
	}
}
