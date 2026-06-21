package gui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"os"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	appservice "github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/routines"
	"github.com/laszukdawid/terminal-agent/internal/tools"
)

const (
	routineLogLimit = 25
	// statusRunning is a GUI-only status shown while a routine launched from the
	// GUI is executing (the persisted statuses are active/inactive/error).
	statusRunning       = "running"
	routineCronExamples = "Examples: 0 9 * * 1-5 (weekdays 9:00) · @daily · @hourly · empty = manual"
	// routineHeaderTitleMax bounds the detail header title so a long name cannot
	// collide with the action buttons on its right.
	routineHeaderTitleMax = 36
	// routineRefreshInterval is how often the Routine tab polls for runs produced
	// out-of-process (the daemon or the CLI) while it is visible.
	routineRefreshInterval = 4 * time.Second
	// routineDaemonOffNotice is shown when scheduled routines exist but the
	// scheduler daemon is not running, so the user knows why they will not fire.
	routineDaemonOffNotice = "Scheduler is not running, so scheduled routines won't fire. Start it with `agent daemon install` (once) or `agent daemon start`."
)

// routineLocalToolNames are the built-in, non-external tools enabled when a
// routine opts into web search via the GUI form (the explicit allow-list then
// also names web search). With no allow-list a routine uses the default policy
// (these local tools on, external off).
var routineLocalToolNames = []string{
	tools.ToolNameRead,
	tools.ToolNameFileSearch,
	tools.ToolNameFileEdit,
	tools.ToolNameUnix,
	tools.ToolNamePython,
}

func (g *App) markRoutineRunning(id string, running bool) {
	g.runningMu.Lock()
	defer g.runningMu.Unlock()
	if g.runningRoutines == nil {
		g.runningRoutines = map[string]bool{}
	}
	if running {
		g.runningRoutines[id] = true
	} else {
		delete(g.runningRoutines, id)
	}
}

func (g *App) isRoutineRunning(id string) bool {
	g.runningMu.Lock()
	defer g.runningMu.Unlock()
	return g.runningRoutines[id]
}

func (g *App) loadRoutines() {
	g.lastRoutineMod = latestRoutineMod()
	views, err := g.routineService.List(context.Background())
	if err != nil {
		g.popup.setRoutines(nil, "Routines unavailable: "+err.Error(), nil, "")
		return
	}
	// Overlay the live "running" status for routines launched from the GUI.
	for i := range views {
		if g.isRoutineRunning(views[i].Routine.ID) {
			views[i].Status = statusRunning
		}
	}
	// Warn when routines are scheduled but no scheduler is running to fire them.
	notice := ""
	if routinesHaveSchedule(views) && !g.routineService.DaemonRunning() {
		notice = routineDaemonOffNotice
	}
	g.popup.setRoutines(views, "", g.showRoutineDetail, notice)
}

// maybeRefreshRoutines reloads the Routine list when it is the visible tab and
// the routine files changed since the last load, so out-of-process runs appear
// without re-navigating, without rebuilding the list when nothing changed.
func (g *App) maybeRefreshRoutines() {
	if !g.state.isVisible || g.state.mode != guiModeRoutine {
		return
	}
	if latestRoutineMod().Equal(g.lastRoutineMod) {
		return
	}
	g.loadRoutines()
}

func routinesHaveSchedule(views []appservice.RoutineView) bool {
	for _, v := range views {
		if v.Routine.Enabled && strings.TrimSpace(v.Routine.Schedule) != "" {
			return true
		}
	}
	return false
}

// latestRoutineMod is the most recent modtime of the routine definitions and run
// state files; it changes whenever a routine is edited or a run is recorded.
func latestRoutineMod() time.Time {
	var latest time.Time
	for _, path := range []string{routines.DefinitionsPath(), routines.StatePath()} {
		if info, err := os.Stat(path); err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest
}

func (p *popupWindow) setRoutines(views []appservice.RoutineView, errorText string, onSelect func(appservice.RoutineView), notice string) {
	if p.routineBody == nil {
		return
	}
	p.routineBody.Objects = nil
	if notice != "" {
		p.routineBody.Add(routineNoticeBanner(notice))
	}
	switch {
	case errorText != "":
		label := widget.NewLabel(errorText)
		label.Wrapping = fyne.TextWrapWord
		p.routineBody.Add(label)
	case len(views) == 0:
		p.routineBody.Add(routineEmptyState())
	default:
		for _, v := range views {
			view := v
			card := newTappableHistoryCard(routineCardContent(view), func() {
				if onSelect != nil {
					onSelect(view)
				}
			})
			p.routineBody.Add(container.New(layout.NewCustomPaddedLayout(0, historyCardGap, 0, 0), card))
		}
	}
	p.routineBody.Refresh()
}

func routineEmptyState() fyne.CanvasObject {
	palette := currentBrandPalette()
	label := canvas.NewText("No routines yet. Use NEW to create one.", palette.secondaryText)
	label.TextSize = theme.TextSize()
	return container.NewPadded(label)
}

// routineNoticeBanner renders a warning banner shown above the routine list (e.g.
// when the scheduler daemon is not running).
func routineNoticeBanner(text string) fyne.CanvasObject {
	palette := currentBrandPalette()
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	label.Importance = widget.WarningImportance
	return container.New(layout.NewCustomPaddedLayout(0, historyCardGap, 0, 0), borderedBox(container.NewPadded(label), palette.warning))
}

func routineCardContent(view appservice.RoutineView) fyne.CanvasObject {
	palette := currentBrandPalette()
	r := view.Routine

	title := canvas.NewText(routineTitle(r), palette.primaryText)
	title.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	title.TextSize = theme.TextSize() - 1
	titleRow := container.NewHBox(vCenter(newStatusDot(routineStatusColor(view.Status))), title)

	meta := canvas.NewText(strings.ToUpper(view.Status)+metaSeparator+view.Frequency+metaSeparator+orDefaultText(r.Model), palette.secondaryText)
	meta.TextSize = theme.TextSize() - 2

	times := canvas.NewText("Last: "+routineTimeText(view.Run.LastRunAt)+metaSeparator+"Next: "+routineTimeText(view.Run.NextRunAt), palette.secondaryText)
	times.TextSize = theme.TextSize() - 2

	prompt := widget.NewLabel(r.PromptPreview(100))
	prompt.Wrapping = fyne.TextWrapWord

	body := container.NewVBox(titleRow, meta, times, brandSeparator(), prompt)
	return borderedBox(body, palette.border)
}

func (g *App) showRoutineDetail(view appservice.RoutineView) {
	palette := currentBrandPalette()
	r := view.Routine

	// Emphasized routine name (large, accent) with the status dot on the left, and
	// the routine actions in the top-right corner of the header. The name is
	// truncated so a long name cannot push into or overlap the action buttons
	// (canvas.Text does not wrap or ellipsize on its own).
	title := canvas.NewText(truncateRunes(routineTitle(r), routineHeaderTitleMax), palette.accentGreen)
	title.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	title.TextSize = theme.TextSize() + 5
	titleLeft := container.NewHBox(vCenter(newStatusDot(routineStatusColor(view.Status))), vCenter(title))
	header := container.NewBorder(nil, nil, titleLeft, vCenter(g.routineHeaderActions(view)), nil)

	settings := fmt.Sprintf(
		"Status: %s\nSchedule: %s\nProvider / Model: %s / %s\nTimeout: %s\nTokens: %s\nTools: %s",
		view.Status, view.Frequency, orDefaultText(r.Provider), orDefaultText(r.Model),
		orDefaultText(r.Timeout), routineBudgetText(r.TokenBudget), routineToolPolicyText(r.Tools),
	)
	if view.HasRun {
		settings += "\nLast run: " + routineTimeText(view.Run.LastRunAt) + " (" + view.Run.LastStatus + ")"
		if view.Run.LastError != "" {
			settings += "\nLast error: " + view.Run.LastError
		}
	}
	settingsLabel := widget.NewLabel(settings)
	settingsLabel.Wrapping = fyne.TextWrapWord

	// The prompt is set apart in its own framed, padded box so it reads as the
	// routine's content rather than more metadata; the label above is clearly a
	// heading, not the first line of the prompt.
	promptBody := widget.NewLabel(r.Prompt)
	promptBody.Wrapping = fyne.TextWrapWord
	promptBox := borderedBox(container.NewPadded(promptBody), palette.borderBright)

	objects := []fyne.CanvasObject{
		header,
		settingsLabel,
		brandSeparator(),
		brandSectionLabel("PROMPT"),
		promptBox,
		brandSeparator(),
		brandSectionLabel("RUN LOGS"),
	}
	objects = append(objects, g.routineLogList(r.ID)...)

	g.popup.presentRoutineDetail(container.NewVBox(objects...))
}

func (g *App) routineLogList(id string) []fyne.CanvasObject {
	refs, err := g.routineService.Logs(context.Background(), id)
	if err != nil {
		return []fyne.CanvasObject{widget.NewLabel("Logs unavailable: " + err.Error())}
	}
	if len(refs) == 0 {
		return []fyne.CanvasObject{widget.NewLabel("No runs recorded yet.")}
	}
	rows := make([]fyne.CanvasObject, 0, routineLogLimit)
	for i, ref := range refs {
		if i >= routineLogLimit {
			break
		}
		ref := ref
		kind := "transcript"
		if ref.IsResult {
			kind = "summary"
		}
		label := routineTimeText(ref.ModTime) + "  ·  " + kind
		rows = append(rows, widget.NewButton(label, func() { g.openRoutineLog(ref) }))
	}
	return rows
}

// routineHeaderActions builds the routine action buttons shown in the top-right
// of the detail header.
func (g *App) routineHeaderActions(view appservice.RoutineView) fyne.CanvasObject {
	r := view.Routine
	runButton := widget.NewButton("Run now", func() { g.runRoutineNow(r.ID) })
	runButton.Importance = widget.HighImportance
	if g.isRoutineRunning(r.ID) {
		runButton.SetText("Running…")
		runButton.Disable()
	}

	toggleLabel := "Disable"
	if !r.Enabled {
		toggleLabel = "Enable"
	}
	toggleButton := widget.NewButton(toggleLabel, func() { g.toggleRoutine(r) })
	editButton := widget.NewButton("Edit", func() {
		g.popup.dismissRoutineDetail()
		routine := r
		g.showRoutineForm(&routine)
	})
	deleteButton := widget.NewButton("Delete", func() { g.confirmDeleteRoutine(r.ID) })
	deleteButton.Importance = widget.DangerImportance

	return container.NewHBox(runButton, toggleButton, editButton, deleteButton)
}

func (g *App) openRoutineLog(ref appservice.RoutineLogRef) {
	data, err := g.routineService.ReadLog(context.Background(), ref)
	if err != nil {
		dialog.ShowError(err, g.popup.window)
		return
	}
	content := container.NewVBox(
		widget.NewLabelWithStyle(ref.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true, Monospace: true}),
		brandSeparator(),
		historyDetailSection("Log", data),
	)
	g.popup.presentRoutineDetail(content)
}

func (g *App) runRoutineNow(id string) {
	g.popup.dismissRoutineDetail()
	// Mark the routine running and refresh the list so its status reads "running"
	// until the (background) run finishes.
	g.markRoutineRunning(id, true)
	if g.state.mode == guiModeRoutine {
		g.loadRoutines()
	}
	go func() {
		_, err := g.routineService.Run(context.Background(), appservice.RoutineRunRequest{
			IDOrName: id,
			Trigger:  routines.TriggerManual,
		})
		g.voiceSchedule(func() {
			g.markRoutineRunning(id, false)
			switch {
			case errors.Is(err, routines.ErrRunInProgress):
				dialog.ShowInformation("Already running", "Routine \""+id+"\" was already running; this run was skipped.", g.popup.window)
			case err != nil:
				dialog.ShowError(err, g.popup.window)
			}
			if g.state.mode == guiModeRoutine {
				g.loadRoutines()
			}
		})
	}()
}

func (g *App) toggleRoutine(r routines.Routine) {
	r.Enabled = !r.Enabled
	if _, err := g.routineService.Save(context.Background(), r); err != nil {
		dialog.ShowError(err, g.popup.window)
		return
	}
	g.popup.dismissRoutineDetail()
	g.loadRoutines()
}

func (g *App) confirmDeleteRoutine(id string) {
	dialog.ShowConfirm("Delete routine", "Delete routine \""+id+"\" and its run history?", func(ok bool) {
		if !ok {
			return
		}
		if err := g.routineService.Delete(context.Background(), id, true); err != nil {
			dialog.ShowError(err, g.popup.window)
			return
		}
		g.popup.dismissRoutineDetail()
		g.loadRoutines()
	}, g.popup.window)
}

// showRoutineForm presents the create/edit dialog. It is built as a brand-themed
// custom dialog (not dialog.NewForm) so the inputs show the native cursor, the
// prompt wraps, the cron field validates live, and the footer buttons are clear.
func (g *App) showRoutineForm(existing *routines.Routine) {
	win := g.popup.window
	isEdit := existing != nil
	var dlg dialog.Dialog

	name := newSettingsTextEntry("")
	name.SetPlaceHolder("Daily standup")
	prompt := widget.NewMultiLineEntry()
	prompt.Wrapping = fyne.TextWrapWord
	prompt.SetMinRowsVisible(5)
	prompt.SetPlaceHolder("What should this routine do?")
	cron := newSettingsTextEntry("")
	cron.SetPlaceHolder("0 9 * * 1-5")
	provider := newSettingsTextEntry("")
	provider.SetPlaceHolder("(default)")
	model := newSettingsTextEntry("")
	model.SetPlaceHolder("(default)")
	timeout := newSettingsTextEntry("")
	timeout.SetPlaceHolder("15m   (0 = unlimited)")
	tokenBudget := newSettingsTextEntry("")
	tokenBudget.SetPlaceHolder("1000000   (0 = unlimited)")
	maxTurns := newSettingsTextEntry("")
	maxTurns.SetPlaceHolder("(default)")
	maxToolCalls := newSettingsTextEntry("")
	maxToolCalls.SetPlaceHolder("(default)")
	deny := widget.NewMultiLineEntry()
	deny.Wrapping = fyne.TextWrapWord
	deny.SetMinRowsVisible(2)
	deny.SetPlaceHolder("one rule per line, e.g. unix(\"rm ...\")")
	webSearch := widget.NewCheck("Allow web search (external tools are off by default)", nil)
	enabled := widget.NewCheck("Enabled", nil)
	enabled.SetChecked(true)

	cronHint := widget.NewLabel(routineCronExamples)
	cronHint.TextStyle = fyne.TextStyle{Italic: true}
	cronHint.Wrapping = fyne.TextWrapWord
	cron.OnChanged = func(value string) {
		if routines.ValidateSchedule(value) != nil {
			cronHint.Importance = widget.DangerImportance
			cronHint.SetText("Invalid cron expression.")
			return
		}
		cronHint.Importance = widget.MediumImportance
		cronHint.SetText(routineCronExamples)
	}

	if isEdit {
		name.SetText(existing.Name)
		prompt.SetText(existing.Prompt)
		cron.SetText(existing.Schedule)
		provider.SetText(existing.Provider)
		model.SetText(existing.Model)
		timeout.SetText(existing.Timeout)
		tokenBudget.SetText(intPtrText(existing.TokenBudget))
		maxTurns.SetText(intPtrText(existing.MaxTurns))
		maxToolCalls.SetText(intPtrText(existing.MaxToolCalls))
		deny.SetText(strings.Join(existing.Deny, "\n"))
		webSearch.SetChecked(routineToolsAllowWebSearch(existing.Tools))
		enabled.SetChecked(existing.Enabled)
	}

	// A routine with a custom tool allow-list (set via the CLI) cannot be
	// represented by the single web-search checkbox; lock the control and preserve
	// the list on save rather than silently flattening it.
	toolsCustom := isEdit && !routineToolsRepresentable(existing.Tools)
	var toolsField fyne.CanvasObject = webSearch
	if toolsCustom {
		webSearch.Disable()
		toolsField = widget.NewLabel("Tools: custom list set via the CLI (unchanged).")
	}

	errorLabel := widget.NewLabel("")
	errorLabel.Wrapping = fyne.TextWrapWord
	errorLabel.Importance = widget.DangerImportance

	form := container.New(layout.NewFormLayout(),
		formFieldLabel("Name"), themedFormField(name),
		formFieldLabel("Prompt"), themedFormField(prompt),
		formFieldLabel("Cron"), themedFormField(cron),
		widget.NewLabel(""), cronHint,
		formFieldLabel("Provider"), themedFormField(provider),
		formFieldLabel("Model"), themedFormField(model),
		formFieldLabel("Timeout"), themedFormField(timeout),
		formFieldLabel("Token budget"), themedFormField(tokenBudget),
		formFieldLabel("Max turns"), themedFormField(maxTurns),
		formFieldLabel("Max tool calls"), themedFormField(maxToolCalls),
		formFieldLabel("Deny rules"), themedFormField(deny),
	)

	save := widget.NewButton("Save", func() {
		routine := routines.Routine{}
		if isEdit {
			routine = *existing
		}
		routine.Name = strings.TrimSpace(name.Text)
		routine.Prompt = strings.TrimSpace(prompt.Text)
		routine.Schedule = strings.TrimSpace(cron.Text)
		routine.Provider = strings.TrimSpace(provider.Text)
		routine.Model = strings.TrimSpace(model.Text)
		routine.Timeout = strings.TrimSpace(timeout.Text)
		routine.Deny = splitNonEmptyLines(deny.Text)
		routine.Enabled = enabled.Checked
		if toolsCustom {
			routine.Tools = existing.Tools
		} else {
			routine.Tools = routineToolsFromWebSearch(webSearch.Checked)
		}

		for _, b := range []struct {
			label string
			text  string
			dest  **int
		}{
			{"Token budget", tokenBudget.Text, &routine.TokenBudget},
			{"Max turns", maxTurns.Text, &routine.MaxTurns},
			{"Max tool calls", maxToolCalls.Text, &routine.MaxToolCalls},
		} {
			value, err := parseOptionalNonNegativeInt(b.text)
			if err != nil {
				errorLabel.SetText(b.label + " " + err.Error() + ".")
				return
			}
			*b.dest = value
		}

		var saveErr error
		if isEdit {
			_, saveErr = g.routineService.Save(context.Background(), routine)
		} else {
			_, saveErr = g.routineService.Create(context.Background(), routine)
		}
		if saveErr != nil {
			errorLabel.SetText(saveErr.Error())
			return
		}
		dlg.Hide()
		if g.state.mode == guiModeRoutine {
			g.loadRoutines()
		}
	})
	save.Importance = widget.HighImportance
	cancel := widget.NewButton("Cancel", func() { dlg.Hide() })
	footer := container.NewBorder(nil, nil, nil, container.NewHBox(cancel, save))

	// Pin the error + Save/Cancel at the bottom and scroll only the fields. Sizing
	// the field scroll to the fields' natural height (capped to the window) keeps
	// the dialog lean (no wasted gap) while the footer stays visible; a tall form
	// scrolls instead of overflowing.
	fields := container.NewVBox(form, toolsField, enabled)
	bottom := container.NewVBox(errorLabel, footer)
	scroll := container.NewVScroll(fields)
	scroll.SetMinSize(fyne.NewSize(routineDialogWidth(win, 560), routineScrollHeight(win, fields, bottom)))
	title := "New routine"
	if isEdit {
		title = "Edit routine"
	}
	dlg = dialog.NewCustomWithoutButtons(title, container.NewBorder(nil, bottom, nil, nil, scroll), win)
	dlg.Resize(fyne.NewSize(routineDialogWidth(win, 600), 0))
	dlg.SetOnClosed(g.FocusInput)
	dlg.Show()
}

// showRoutineDefaultsForm edits the defaults applied to routines that do not set
// their own values, plus the global routines on/off toggle. Reached from
// Settings → "Routine defaults…".
func (g *App) showRoutineDefaultsForm() {
	win := g.popup.window
	defaults := g.cfg.GetRoutineDefaults()
	var dlg dialog.Dialog

	enabled := widget.NewCheck("Routines enabled", nil)
	enabled.SetChecked(g.cfg.GetRoutinesEnabled())
	provider := newSettingsTextEntry(defaults.Provider)
	provider.SetPlaceHolder("(global provider)")
	model := newSettingsTextEntry(defaults.Model)
	model.SetPlaceHolder("(provider default)")
	timeout := newSettingsTextEntry(defaults.Timeout)
	timeout.SetPlaceHolder("15m")
	tokenBudget := newSettingsTextEntry(intPtrText(defaults.TokenBudget))
	tokenBudget.SetPlaceHolder("1000000")
	maxTurns := newSettingsTextEntry(intPtrText(defaults.MaxTurns))
	maxToolCalls := newSettingsTextEntry(intPtrText(defaults.MaxToolCalls))

	errorLabel := widget.NewLabel("")
	errorLabel.Wrapping = fyne.TextWrapWord
	errorLabel.Importance = widget.DangerImportance

	form := container.New(layout.NewFormLayout(),
		formFieldLabel("Provider"), themedFormField(provider),
		formFieldLabel("Model"), themedFormField(model),
		formFieldLabel("Timeout"), themedFormField(timeout),
		formFieldLabel("Token budget"), themedFormField(tokenBudget),
		formFieldLabel("Max turns"), themedFormField(maxTurns),
		formFieldLabel("Max tool calls"), themedFormField(maxToolCalls),
	)

	save := widget.NewButton("Save", func() {
		updated := config.RoutineDefaults{
			Provider: strings.TrimSpace(provider.Text),
			Model:    strings.TrimSpace(model.Text),
			Timeout:  strings.TrimSpace(timeout.Text),
		}
		for _, b := range []struct {
			label string
			text  string
			dest  **int
		}{
			{"Token budget", tokenBudget.Text, &updated.TokenBudget},
			{"Max turns", maxTurns.Text, &updated.MaxTurns},
			{"Max tool calls", maxToolCalls.Text, &updated.MaxToolCalls},
		} {
			value, err := parseOptionalNonNegativeInt(b.text)
			if err != nil {
				errorLabel.SetText(b.label + " " + err.Error() + ".")
				return
			}
			*b.dest = value
		}
		if err := g.cfg.SetRoutinesEnabled(enabled.Checked); err != nil {
			errorLabel.SetText(err.Error())
			return
		}
		if err := g.cfg.SetRoutineDefaults(updated); err != nil {
			errorLabel.SetText(err.Error())
			return
		}
		dlg.Hide()
	})
	save.Importance = widget.HighImportance
	cancel := widget.NewButton("Cancel", func() { dlg.Hide() })
	footer := container.NewBorder(nil, nil, nil, container.NewHBox(cancel, save))

	fields := container.NewVBox(enabled, form)
	bottom := container.NewVBox(errorLabel, footer)
	scroll := container.NewVScroll(fields)
	scroll.SetMinSize(fyne.NewSize(routineDialogWidth(win, 480), routineScrollHeight(win, fields, bottom)))
	dlg = dialog.NewCustomWithoutButtons("Routine defaults", container.NewBorder(nil, bottom, nil, nil, scroll), win)
	dlg.Resize(fyne.NewSize(routineDialogWidth(win, 520), 0))
	dlg.Show()
}

func (p *popupWindow) presentRoutineDetail(content fyne.CanvasObject) {
	p.dismissRoutineDetail()
	size := historyDetailPopupSize(p.window.Canvas().Size())
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(size.Width, max(120, size.Height-historyDetailFooterHeight)))
	closeButton := widget.NewButton("Close", nil)
	detail := borderedBox(container.NewBorder(nil, closeButton, nil, nil, scroll), currentBrandPalette().border)
	overlay := newHistoryDetailOverlay(detail, size, nil)
	pop := widget.NewPopUp(overlay, p.window.Canvas())
	overlay.onDismiss = func() { p.dismissRoutineDetail() }
	closeButton.OnTapped = func() { p.dismissRoutineDetail() }
	pop.Resize(p.window.Canvas().Size())
	p.routineDetail = pop
	p.window.Canvas().Unfocus()
	pop.ShowAtPosition(fyne.NewPos(0, 0))
}

func (p *popupWindow) dismissRoutineDetail() bool {
	if p.routineDetail == nil || !p.routineDetail.Visible() {
		return false
	}
	p.routineDetail.Hide()
	return true
}

// --- helpers ---

// themedFormField wraps a form input so it picks up the prompt entry theme, which
// restores the native text cursor that the brand theme otherwise hides by zeroing
// the input border.
func themedFormField(o fyne.CanvasObject) fyne.CanvasObject {
	return container.NewThemeOverride(o, promptEntryTheme{Theme: newBrandTheme()})
}

func formFieldLabel(text string) fyne.CanvasObject {
	return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

// routineScrollHeight sizes the scrolling field area to the fields' natural
// height so the dialog hugs the content (no wasted space above the pinned
// footer), capped at the window height (less the footer) so a tall form scrolls
// instead of overflowing on a small window.
func routineScrollHeight(win fyne.Window, fields, footer fyne.CanvasObject) float32 {
	available := win.Canvas().Size().Height - 120 - footer.MinSize().Height
	if available < 120 {
		available = 120
	}
	natural := fields.MinSize().Height
	if natural > available {
		return available
	}
	return natural
}

// routineDialogWidth caps a preferred dialog width to the window so the dialog
// never renders wider than the screen.
func routineDialogWidth(win fyne.Window, preferred float32) float32 {
	available := win.Canvas().Size().Width - 40
	if preferred > available {
		return available
	}
	return preferred
}

// truncateRunes shortens s to at most n runes, appending an ellipsis when cut.
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return strings.TrimSpace(string(runes[:n])) + "…"
}

// parseOptionalNonNegativeInt parses an optional integer field that must be zero
// or positive (the budgets/limits use 0 to mean unlimited/default). It returns
// user-facing error text suitable for prefixing with the field label.
func parseOptionalNonNegativeInt(text string) (*int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		return nil, fmt.Errorf("must be a number")
	}
	if value < 0 {
		return nil, fmt.Errorf("must be zero or a positive number")
	}
	return &value, nil
}

func routineTitle(r routines.Routine) string {
	if strings.TrimSpace(r.Name) != "" {
		return r.Name
	}
	return r.ID
}

func routineStatusColor(status string) color.Color {
	palette := currentBrandPalette()
	switch status {
	case routines.StatusError:
		return palette.error
	case routines.StatusInactive:
		return palette.disabledText
	case statusRunning:
		return palette.warning
	default:
		return palette.accentGreen
	}
}

func routineTimeText(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func orDefaultText(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(default)"
	}
	return value
}

func routineBudgetText(budget *int) string {
	if budget == nil {
		return "(default)"
	}
	if *budget <= 0 {
		return "unlimited"
	}
	return strconv.Itoa(*budget)
}

func routineToolPolicyText(toolsList []string) string {
	if toolsList == nil {
		return "default (external-facing disabled)"
	}
	if len(toolsList) == 0 {
		return "(none)"
	}
	return strings.Join(toolsList, ", ")
}

// routineToolsRepresentable reports whether a tool allow-list can be round-tripped
// through the GUI's single web-search checkbox: nil (default policy) or exactly the
// local tools plus web search. Anything else is a custom list edited via the CLI.
func routineToolsRepresentable(toolsList []string) bool {
	if toolsList == nil {
		return true
	}
	return equalStringSet(toolsList, routineToolsFromWebSearch(true))
}

func equalStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, s := range a {
		counts[s]++
	}
	for _, s := range b {
		counts[s]--
	}
	for _, c := range counts {
		if c != 0 {
			return false
		}
	}
	return true
}

func routineToolsAllowWebSearch(toolsList []string) bool {
	for _, name := range toolsList {
		if name == tools.ToolNameWebsearch {
			return true
		}
	}
	return false
}

// routineToolsFromWebSearch maps the GUI's single web-search toggle to the
// routine's tool allow-list: off keeps the default policy (nil); on enables the
// local tools plus web search explicitly.
func routineToolsFromWebSearch(allow bool) []string {
	if !allow {
		return nil
	}
	enabled := make([]string, 0, len(routineLocalToolNames)+1)
	enabled = append(enabled, routineLocalToolNames...)
	enabled = append(enabled, tools.ToolNameWebsearch)
	return enabled
}

func splitNonEmptyLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseOptionalInt(text string) (*int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func intPtrText(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func ptr[T any](v T) *T { return &v }
