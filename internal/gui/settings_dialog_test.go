package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
)

// fakeDialog is a dialog.Dialog stub that records whether Hide was called so the
// Escape-to-close logic can be tested without rendering a real window.
type fakeDialog struct {
	hidden bool
}

func (f *fakeDialog) Show()                 {}
func (f *fakeDialog) Hide()                 { f.hidden = true }
func (f *fakeDialog) SetDismissText(string) {}
func (f *fakeDialog) SetOnClosed(func())    {}
func (f *fakeDialog) Refresh()              {}
func (f *fakeDialog) Resize(fyne.Size)      {}
func (f *fakeDialog) MinSize() fyne.Size    { return fyne.Size{} }
func (f *fakeDialog) Dismiss()              {}

var _ dialog.Dialog = (*fakeDialog)(nil)

func TestSettingsDialogChanged(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	tests := []struct {
		name             string
		baselineProvider string
		baselineModel    string
		provider         string
		model            string
		want             bool
	}{
		{name: "no edits", baselineProvider: "openai", baselineModel: "gpt-4o", provider: "openai", model: "gpt-4o", want: false},
		{name: "provider edited", baselineProvider: "openai", baselineModel: "gpt-4o", provider: "anthropic", model: "gpt-4o", want: true},
		{name: "model edited", baselineProvider: "openai", baselineModel: "gpt-4o", provider: "openai", model: "gpt-4o-mini", want: true},
		{name: "both edited", baselineProvider: "openai", baselineModel: "gpt-4o", provider: "anthropic", model: "claude", want: true},
		{name: "whitespace only is not a change", baselineProvider: "openai", baselineModel: "gpt-4o", provider: "  openai  ", model: " gpt-4o ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &settingsDialog{
				provider:         newProviderEntry(tt.provider, nil),
				model:            newSettingsTextEntry(tt.model),
				baselineProvider: tt.baselineProvider,
				baselineModel:    tt.baselineModel,
			}
			assert.Equal(t, tt.want, s.changed())
		})
	}
}

func TestSettingsTextEntryEscapeRouting(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	tests := []struct {
		name        string
		key         fyne.KeyName
		wantEscaped bool
	}{
		{name: "escape calls onEscape", key: fyne.KeyEscape, wantEscaped: true},
		{name: "other key does not", key: fyne.KeyA, wantEscaped: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			escaped := false
			entry := newSettingsTextEntry("gpt-4o")
			entry.onEscape = func() { escaped = true }
			entry.TypedKey(&fyne.KeyEvent{Name: tt.key})
			assert.Equal(t, tt.wantEscaped, escaped)
		})
	}
}

func TestProviderEntryEscapeRouting(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	tests := []struct {
		name        string
		key         fyne.KeyName
		wantEscaped bool
	}{
		{name: "escape calls onEscape", key: fyne.KeyEscape, wantEscaped: true},
		{name: "other key does not", key: fyne.KeyA, wantEscaped: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			escaped := false
			entry := newProviderEntry("openai", nil)
			entry.onEscape = func() { escaped = true }
			entry.TypedKey(&fyne.KeyEvent{Name: tt.key})
			assert.Equal(t, tt.wantEscaped, escaped)
		})
	}
}

// TestDismissSettingsIfUnchanged checks the Escape-to-close decision: an
// unedited panel hides, an edited panel stays open, and a closed panel reports
// the key as unhandled.
func TestDismissSettingsIfUnchanged(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	newOpenPanel := func(model string) (*popupWindow, *fakeDialog) {
		fd := &fakeDialog{}
		p := &popupWindow{settings: &settingsDialog{
			dialog:           fd,
			provider:         newProviderEntry("openai", nil),
			model:            newSettingsTextEntry(model),
			baselineProvider: "openai",
			baselineModel:    "gpt-4o",
		}}
		return p, fd
	}

	t.Run("unchanged panel hides and reports handled", func(t *testing.T) {
		p, fd := newOpenPanel("gpt-4o")
		assert.True(t, p.dismissSettingsIfUnchanged())
		assert.True(t, fd.hidden, "an unchanged panel should hide on Escape")
	})

	t.Run("edited panel stays open but reports handled", func(t *testing.T) {
		p, fd := newOpenPanel("gpt-4o-mini")
		assert.True(t, p.dismissSettingsIfUnchanged())
		assert.False(t, fd.hidden, "an edited panel must stay open on Escape")
	})

	t.Run("no panel reports unhandled", func(t *testing.T) {
		p := &popupWindow{}
		assert.False(t, p.dismissSettingsIfUnchanged())
	})
}
