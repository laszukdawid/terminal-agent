package gui

import (
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/laszukdawid/terminal-agent/internal/connector"
)

type settingsDialogOptions struct {
	InitialProvider  string
	InitialModel     string
	Version          string
	EnvResult        EnvironmentLoadResult
	ModelForProvider func(provider string) string
	OnSave           func(provider, model string) error
	OnClosed         func()
}

func (p *popupWindow) showSettingsDialog(options settingsDialogOptions) {
	win := p.window
	currentEnvResult := options.EnvResult
	modelEntry := newSettingsTextEntry(options.InitialModel)
	errorLabel := widget.NewLabel("")
	errorLabel.Wrapping = fyne.TextWrapWord
	errorLabel.Importance = widget.DangerImportance

	// Hints are always present and single-line so changing their text never
	// changes the dialog height. A height change would resize and recenter the
	// dialog, moving the provider field without moving the already-open
	// autocomplete popup, leaving the popup covering the field's text.
	newHint := func() *widget.Label {
		l := widget.NewLabel("")
		l.Wrapping = fyne.TextWrapOff
		l.Truncation = fyne.TextTruncateEllipsis
		l.TextStyle = fyne.TextStyle{Italic: true}
		return l
	}
	modelHint := newHint()
	providerStatus := newProviderStatusIcon(win.Canvas())

	updateHints := func(provider string) {
		provider = strings.TrimSpace(provider)
		providerStatus.setStatus(providerReadinessStatusWithEnvironment(provider, currentEnvResult))
		if options.ModelForProvider != nil && slices.Contains(connector.SupportedProviders(), provider) {
			if model := strings.TrimSpace(options.ModelForProvider(provider)); model != "" {
				modelEntry.SetText(model)
			}
		}
		if def := connector.DefaultModelFor(provider); def != "" {
			modelHint.SetText("Default model: " + def)
		} else {
			modelHint.SetText("")
		}
	}

	providerInput := newProviderEntry(options.InitialProvider, updateHints)
	providerInputField := container.NewThemeOverride(providerInput, promptEntryTheme{Theme: newBrandTheme()})
	modelInputField := container.NewThemeOverride(modelEntry, promptEntryTheme{Theme: newBrandTheme()})

	providerLabel := widget.NewLabelWithStyle("Provider", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	providerLabelBox := container.NewHBox(providerLabel, providerStatus)
	modelLabel := widget.NewLabelWithStyle("Model", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// A single FormLayout grid keeps the Provider and Model labels and inputs
	// aligned, and makes both inputs start at the same x with the same width.
	// Hint rows use an empty label cell so each hint aligns under its input.
	form := container.New(layout.NewFormLayout(),
		providerLabelBox, providerInputField,
		modelLabel, modelInputField,
		widget.NewLabel(""), modelHint,
	)
	environmentSummary := environmentSummaryText(currentEnvResult)
	var dlg dialog.Dialog
	saveButton := widget.NewButton("Save", func() {
		provider := strings.TrimSpace(providerInput.Text)
		model := strings.TrimSpace(modelEntry.Text)
		if provider == "" {
			errorLabel.SetText("Provider cannot be empty.")
			return
		}
		if model == "" {
			errorLabel.SetText("Model cannot be empty.")
			return
		}
		if options.OnSave == nil {
			errorLabel.SetText("Settings cannot be saved.")
			return
		}
		if err := options.OnSave(provider, model); err != nil {
			errorLabel.SetText(err.Error())
			return
		}
		dlg.Hide()
	})
	saveButton.Importance = widget.HighImportance
	cancelButton := widget.NewButton("Cancel", func() {
		dlg.Hide()
	})
	versionLabel := widget.NewLabelWithStyle(options.Version, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	footer := container.NewHBox(versionLabel, layout.NewSpacer(), cancelButton, saveButton)
	contentObjects := []fyne.CanvasObject{form}
	if environmentSummary != "" {
		environmentLabel := widget.NewLabel(environmentSummary)
		environmentLabel.Wrapping = fyne.TextWrapWord
		contentObjects = append(contentObjects,
			widget.NewLabelWithStyle("Environment", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			environmentLabel,
		)
	}
	contentObjects = append(contentObjects, errorLabel, footer)
	content := container.NewVBox(contentObjects...)
	updateHints(options.InitialProvider)

	dlg = dialog.NewCustomWithoutButtons("Settings", content, win)

	// Escape closes the panel only when nothing has been edited; pending edits
	// must be resolved explicitly via Save or Cancel so they are never lost to a
	// stray keypress. The baseline is captured after updateHints so a value
	// auto-populated while building the dialog does not count as an edit.
	settings := &settingsDialog{
		dialog:           dlg,
		provider:         providerInput,
		model:            modelEntry,
		baselineProvider: providerInput.Text,
		baselineModel:    modelEntry.Text,
	}
	providerInput.onEscape = func() { p.dismissSettingsIfUnchanged() }
	modelEntry.onEscape = func() { p.dismissSettingsIfUnchanged() }
	p.settings = settings
	dlg.SetOnClosed(func() {
		p.settings = nil
		if options.OnClosed != nil {
			options.OnClosed()
		}
	})
	dlg.Resize(fyne.NewSize(520, 0))
	dlg.Show()
}

// settingsDialog tracks an open Settings panel so Escape can dismiss it only
// when the user has not edited any field.
type settingsDialog struct {
	dialog           dialog.Dialog
	provider         *providerEntry
	model            *settingsTextEntry
	baselineProvider string
	baselineModel    string
}

// changed reports whether the provider or model differs from the values shown
// when the dialog opened, ignoring surrounding whitespace.
func (s *settingsDialog) changed() bool {
	return strings.TrimSpace(s.provider.Text) != strings.TrimSpace(s.baselineProvider) ||
		strings.TrimSpace(s.model.Text) != strings.TrimSpace(s.baselineModel)
}

// dismissSettingsIfUnchanged closes the Settings dialog on Escape when it is
// open and has no unsaved edits. It returns true when a Settings dialog is open
// so the caller treats Escape as handled; pending edits leave the dialog open
// to be resolved via Save or Cancel.
func (p *popupWindow) dismissSettingsIfUnchanged() bool {
	if p.settings == nil {
		return false
	}
	if !p.settings.changed() {
		p.settings.dialog.Hide()
	}
	return true
}

func environmentSummaryText(result EnvironmentLoadResult) string {
	lines := []string{}
	if result.EnvFileError != nil {
		lines = append(lines, "App env file: "+result.EnvFileError.Error())
	}
	if result.EnvFileWarning != nil {
		lines = append(lines, "App env file warning: "+result.EnvFileWarning.Error())
	}
	if result.ShellError != nil {
		lines = append(lines, "Shell import failed: "+result.ShellError.Error())
	}
	return strings.Join(lines, "\n")
}
