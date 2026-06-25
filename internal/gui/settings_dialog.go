package gui

import (
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
)

// Info-icon messages explaining each routine default. They describe what the
// setting controls, while the field's placeholder shows the value that applies
// when the field is left blank.
const (
	routineProviderInfo     = "Provider used for routine runs that do not set their own. Leave blank to fall back to the global provider."
	routineModelInfo        = "Model used for routine runs that do not set their own. Leave blank to fall back to the provider's default model."
	routineTimeoutInfo      = "Wall-clock limit for a single routine run (e.g. 30s, 15m or 1h). 0 means no timeout."
	routineTokenBudgetInfo  = "Maximum estimated tokens a routine run may use. 0 means unlimited."
	routineMaxTurnsInfo     = "Maximum number of agent turns (model round-trips) allowed in a single routine run."
	routineMaxToolCallsInfo = "Maximum number of tool calls allowed in a single routine run."
)

// routineDefaultHints carries the placeholder values shown for the routine
// default fields whose fallback is a fixed product/agent default (rather than a
// value derived from the chosen provider).
type routineDefaultHints struct {
	Timeout      string
	TokenBudget  string
	MaxTurns     string
	MaxToolCalls string
}

type settingsDialogOptions struct {
	InitialProvider        string
	InitialModel           string
	Version                string
	EnvResult              EnvironmentLoadResult
	ModelForProvider       func(provider string) string
	OnSave                 func(provider, model string) error
	InitialRoutineDefaults config.RoutineDefaults
	RoutineDefaultHints    routineDefaultHints
	OnSaveRoutineDefaults  func(config.RoutineDefaults) error
	OnClosed               func()
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

	// Routine default fields. A blank field means "use the default", and that
	// default is shown as the field's placeholder so the user sees it without it
	// becoming an explicit override saved back to config.
	rd := options.InitialRoutineDefaults
	routineProvider := newSettingsTextEntry(rd.Provider)
	routineModel := newSettingsTextEntry(rd.Model)
	routineTimeout := newSettingsTextEntry(rd.Timeout)
	routineTimeout.SetPlaceHolder(defaultHintText(options.RoutineDefaultHints.Timeout))
	routineTokenBudget := newSettingsTextEntry(intPtrText(rd.TokenBudget))
	routineTokenBudget.SetPlaceHolder(defaultHintText(options.RoutineDefaultHints.TokenBudget))
	routineMaxTurns := newSettingsTextEntry(intPtrText(rd.MaxTurns))
	routineMaxTurns.SetPlaceHolder(defaultHintText(options.RoutineDefaultHints.MaxTurns))
	routineMaxToolCalls := newSettingsTextEntry(intPtrText(rd.MaxToolCalls))
	routineMaxToolCalls.SetPlaceHolder(defaultHintText(options.RoutineDefaultHints.MaxToolCalls))

	// Declared up front so the hint closures below can read the live global
	// provider value before the entry itself is constructed.
	var providerInput *providerEntry

	// updateRoutineModelHint keeps the routine Model placeholder showing the model a
	// blank field would resolve to: the routine Provider override when set, else the
	// global provider. This mirrors how a run resolves a routine's model.
	updateRoutineModelHint := func() {
		effective := strings.TrimSpace(routineProvider.Text)
		if effective == "" && providerInput != nil {
			effective = strings.TrimSpace(providerInput.Text)
		}
		model := ""
		if options.ModelForProvider != nil && effective != "" {
			model = strings.TrimSpace(options.ModelForProvider(effective))
		}
		routineModel.SetPlaceHolder(routineModelHint(model))
	}
	routineProvider.OnChanged = func(string) { updateRoutineModelHint() }

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
		// The routine Provider default falls back to the global provider, so its
		// placeholder tracks the live provider value edited above; the Model
		// placeholder tracks whichever provider the routine would actually use.
		routineProvider.SetPlaceHolder(routineProviderHint(provider))
		updateRoutineModelHint()
	}

	providerInput = newProviderEntry(options.InitialProvider, updateHints)
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

	// Routines section: the defaults applied to routines that do not set their own
	// values, inline rather than behind a separate dialog. Each label carries an
	// info icon explaining the setting.
	routineForm := container.New(layout.NewFormLayout(),
		settingsFieldLabel(win, "Provider", routineProviderInfo), themedFormField(routineProvider),
		settingsFieldLabel(win, "Model", routineModelInfo), themedFormField(routineModel),
		settingsFieldLabel(win, "Timeout", routineTimeoutInfo), themedFormField(routineTimeout),
		settingsFieldLabel(win, "Token budget", routineTokenBudgetInfo), themedFormField(routineTokenBudget),
		settingsFieldLabel(win, "Max turns", routineMaxTurnsInfo), themedFormField(routineMaxTurns),
		settingsFieldLabel(win, "Max tool calls", routineMaxToolCallsInfo), themedFormField(routineMaxToolCalls),
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
		// Collect the routine defaults first so a malformed numeric field is caught
		// before anything is persisted, avoiding a partial save.
		routineDefaults := config.RoutineDefaults{
			Provider: strings.TrimSpace(routineProvider.Text),
			Model:    strings.TrimSpace(routineModel.Text),
			Timeout:  strings.TrimSpace(routineTimeout.Text),
		}
		for _, b := range []struct {
			label string
			text  string
			dest  **int
		}{
			{"Token budget", routineTokenBudget.Text, &routineDefaults.TokenBudget},
			{"Max turns", routineMaxTurns.Text, &routineDefaults.MaxTurns},
			{"Max tool calls", routineMaxToolCalls.Text, &routineDefaults.MaxToolCalls},
		} {
			value, err := parseOptionalNonNegativeInt(b.text)
			if err != nil {
				errorLabel.SetText(b.label + " " + err.Error() + ".")
				return
			}
			*b.dest = value
		}
		if err := options.OnSave(provider, model); err != nil {
			errorLabel.SetText(err.Error())
			return
		}
		if options.OnSaveRoutineDefaults != nil {
			if err := options.OnSaveRoutineDefaults(routineDefaults); err != nil {
				errorLabel.SetText(err.Error())
				return
			}
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
			brandSeparator(),
			settingsSectionHeading("Environment"),
			environmentLabel,
		)
	}
	contentObjects = append(contentObjects,
		brandSeparator(),
		settingsSectionHeading("Routines"),
		routineForm,
		errorLabel,
		footer,
	)
	content := container.NewVBox(contentObjects...)
	updateHints(options.InitialProvider)

	dlg = dialog.NewCustomWithoutButtons("Settings", content, win)

	// Escape closes the panel only when nothing has been edited; pending edits
	// must be resolved explicitly via Save or Cancel so they are never lost to a
	// stray keypress. Baselines are captured after updateHints so a value
	// auto-populated while building the dialog does not count as an edit.
	track := func(get func() string) trackedField {
		return trackedField{current: get, baseline: get()}
	}
	settings := &settingsDialog{
		dialog: dlg,
		fields: []trackedField{
			track(func() string { return providerInput.Text }),
			track(func() string { return modelEntry.Text }),
			track(func() string { return routineProvider.Text }),
			track(func() string { return routineModel.Text }),
			track(func() string { return routineTimeout.Text }),
			track(func() string { return routineTokenBudget.Text }),
			track(func() string { return routineMaxTurns.Text }),
			track(func() string { return routineMaxToolCalls.Text }),
		},
	}
	dismiss := func() { p.dismissSettingsIfUnchanged() }
	providerInput.onEscape = dismiss
	modelEntry.onEscape = dismiss
	routineProvider.onEscape = dismiss
	routineModel.onEscape = dismiss
	routineTimeout.onEscape = dismiss
	routineTokenBudget.onEscape = dismiss
	routineMaxTurns.onEscape = dismiss
	routineMaxToolCalls.onEscape = dismiss
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

// settingsFieldLabel builds a bold form-row label paired with a tappable info
// icon that explains the setting on click. The icon is pinned to the right of the
// label cell (which FormLayout stretches to the label column width), so every
// icon sits the same short distance to the left of its input.
func settingsFieldLabel(win fyne.Window, text, info string) fyne.CanvasObject {
	return container.NewBorder(nil, nil, formFieldLabel(text), newInfoIcon(win.Canvas(), info))
}

// settingsSectionHeading renders a settings group title in the brand accent so it
// reads as a divider between groups rather than another form-field label.
func settingsSectionHeading(text string) fyne.CanvasObject {
	palette := currentBrandPalette()
	t := canvas.NewText(text, palette.accentGreen)
	t.TextStyle = fyne.TextStyle{Bold: true}
	t.TextSize = theme.TextSize() + 1
	return t
}

// defaultHintText renders a field placeholder that names the default applied when
// the field is left blank, e.g. "default: 15m". An empty value yields no hint.
func defaultHintText(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "default: " + value
}

// routineProviderHint is the placeholder for the routine default provider: it
// names the global provider the run falls back to when the field is blank.
func routineProviderHint(provider string) string {
	if strings.TrimSpace(provider) == "" {
		return "(global provider)"
	}
	return "default: " + provider
}

// routineModelHint is the placeholder for the routine default model: it names the
// provider's default model the run falls back to when the field is blank.
func routineModelHint(model string) string {
	if strings.TrimSpace(model) == "" {
		return "(provider default)"
	}
	return "default: " + model
}

// trackedField pairs a live text accessor with the value shown when the Settings
// dialog opened, so the dialog can tell whether a field has unsaved edits.
type trackedField struct {
	current  func() string
	baseline string
}

// settingsDialog tracks an open Settings panel so Escape can dismiss it only when
// the user has not edited any field.
type settingsDialog struct {
	dialog dialog.Dialog
	fields []trackedField
}

// changed reports whether any tracked field differs from the value shown when the
// dialog opened, ignoring surrounding whitespace.
func (s *settingsDialog) changed() bool {
	for _, f := range s.fields {
		if strings.TrimSpace(f.current()) != strings.TrimSpace(f.baseline) {
			return true
		}
	}
	return false
}

// dismissSettingsIfUnchanged closes the Settings dialog on Escape when it is open
// and has no unsaved edits. It returns true when a Settings dialog is open so the
// caller treats Escape as handled; pending edits leave the dialog open to be
// resolved via Save or Cancel.
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
