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

const routineLogLimit = 25

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

func (g *App) loadRoutines() {
	views, err := g.routineService.List(context.Background())
	if err != nil {
		g.popup.setRoutines(nil, "Routines unavailable: "+err.Error(), nil)
		return
	}
	g.popup.setRoutines(views, "", g.showRoutineDetail)
}

func (p *popupWindow) setRoutines(views []appservice.RoutineView, errorText string, onSelect func(appservice.RoutineView)) {
	if p.routineBody == nil {
		return
	}
	p.routineBody.Objects = nil
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
	r := view.Routine
	info := fmt.Sprintf(
		"Status: %s\nSchedule: %s\nProvider / Model: %s / %s\nTimeout: %s\nTokens: %s\nTools: %s",
		view.Status, view.Frequency, orDefaultText(r.Provider), orDefaultText(r.Model),
		orDefaultText(r.Timeout), routineBudgetText(r.TokenBudget), routineToolPolicyText(r.Tools),
	)
	if view.HasRun {
		info += "\nLast run: " + routineTimeText(view.Run.LastRunAt) + " (" + view.Run.LastStatus + ")"
		if view.Run.LastError != "" {
			info += "\nLast error: " + view.Run.LastError
		}
	}
	infoLabel := widget.NewLabel(info)
	infoLabel.Wrapping = fyne.TextWrapWord

	objects := []fyne.CanvasObject{
		widget.NewLabelWithStyle(routineTitle(r), fyne.TextAlignLeading, fyne.TextStyle{Bold: true, Monospace: true}),
		infoLabel,
		brandSeparator(),
		historyDetailSection("Prompt", r.Prompt),
		widget.NewLabelWithStyle("Run logs", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	}
	objects = append(objects, g.routineLogList(r.ID)...)
	objects = append(objects, brandSeparator(), g.routineActionBar(view))

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

func (g *App) routineActionBar(view appservice.RoutineView) fyne.CanvasObject {
	r := view.Routine
	runButton := widget.NewButton("Run now", func() { g.runRoutineNow(r.ID) })
	runButton.Importance = widget.HighImportance

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

	return container.NewHBox(runButton, toggleButton, editButton, layout.NewSpacer(), deleteButton)
}

func (g *App) openRoutineLog(ref appservice.RoutineLogRef) {
	data, err := os.ReadFile(ref.Path)
	if err != nil {
		dialog.ShowError(err, g.popup.window)
		return
	}
	content := container.NewVBox(
		widget.NewLabelWithStyle(ref.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true, Monospace: true}),
		brandSeparator(),
		historyDetailSection("Log", string(data)),
	)
	g.popup.presentRoutineDetail(content)
}

func (g *App) runRoutineNow(id string) {
	g.popup.dismissRoutineDetail()
	go func() {
		_, err := g.routineService.Run(context.Background(), appservice.RoutineRunRequest{
			IDOrName: id,
			Trigger:  routines.TriggerManual,
		})
		g.voiceSchedule(func() {
			if err != nil && !errors.Is(err, routines.ErrRunInProgress) {
				dialog.ShowError(err, g.popup.window)
			}
			if g.state.mode == guiModeRoutine {
				g.loadRoutines()
			}
		})
	}()
	dialog.ShowInformation("Routine started", "Running \""+id+"\". The list updates when it finishes.", g.popup.window)
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

func (g *App) showRoutineForm(existing *routines.Routine) {
	isEdit := existing != nil

	name := widget.NewEntry()
	prompt := widget.NewMultiLineEntry()
	prompt.SetMinRowsVisible(4)
	cron := widget.NewEntry()
	cron.SetPlaceHolder("0 9 * * 1-5   (empty = manual only)")
	provider := widget.NewEntry()
	provider.SetPlaceHolder("(default)")
	model := widget.NewEntry()
	model.SetPlaceHolder("(default)")
	timeout := widget.NewEntry()
	timeout.SetPlaceHolder("15m   (0 = unlimited)")
	tokenBudget := widget.NewEntry()
	tokenBudget.SetPlaceHolder("1000000   (0 = unlimited)")
	maxTurns := widget.NewEntry()
	maxToolCalls := widget.NewEntry()
	deny := widget.NewMultiLineEntry()
	deny.SetPlaceHolder("one deny rule per line, e.g. unix(\"rm ...\")")
	webSearch := widget.NewCheck("Allow web search (and other external tools off by default)", nil)
	enabled := widget.NewCheck("Enabled", nil)
	enabled.SetChecked(true)

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

	items := []*widget.FormItem{
		widget.NewFormItem("Name", name),
		widget.NewFormItem("Prompt", prompt),
		widget.NewFormItem("Cron", cron),
		widget.NewFormItem("Provider", provider),
		widget.NewFormItem("Model", model),
		widget.NewFormItem("Timeout", timeout),
		widget.NewFormItem("Token budget", tokenBudget),
		widget.NewFormItem("Max turns", maxTurns),
		widget.NewFormItem("Max tool calls", maxToolCalls),
		widget.NewFormItem("Deny rules", deny),
		widget.NewFormItem("", webSearch),
		widget.NewFormItem("", enabled),
	}

	title := "New routine"
	if isEdit {
		title = "Edit routine"
	}
	formDialog := dialog.NewForm(title, "Save", "Cancel", items, func(ok bool) {
		if !ok {
			return
		}
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
		routine.Tools = routineToolsFromWebSearch(webSearch.Checked)

		budgets := map[string]*string{
			"token budget":   ptr(tokenBudget.Text),
			"max turns":      ptr(maxTurns.Text),
			"max tool calls": ptr(maxToolCalls.Text),
		}
		parsed := map[string]**int{
			"token budget":   &routine.TokenBudget,
			"max turns":      &routine.MaxTurns,
			"max tool calls": &routine.MaxToolCalls,
		}
		for label, raw := range budgets {
			value, err := parseOptionalInt(*raw)
			if err != nil {
				dialog.ShowError(fmt.Errorf("%s must be a number", label), g.popup.window)
				return
			}
			*parsed[label] = value
		}

		var saveErr error
		if isEdit {
			_, saveErr = g.routineService.Save(context.Background(), routine)
		} else {
			_, saveErr = g.routineService.Create(context.Background(), routine)
		}
		if saveErr != nil {
			dialog.ShowError(saveErr, g.popup.window)
			return
		}
		if g.state.mode == guiModeRoutine {
			g.loadRoutines()
		}
	}, g.popup.window)
	formDialog.Resize(fyne.NewSize(560, 0))
	formDialog.Show()
}

// showRoutineDefaultsForm edits the defaults applied to routines that do not set
// their own values, plus the global routines on/off toggle. It is reached from
// Settings → "Routine defaults…".
func (g *App) showRoutineDefaultsForm() {
	defaults := g.cfg.GetRoutineDefaults()

	enabled := widget.NewCheck("Routines enabled", nil)
	enabled.SetChecked(g.cfg.GetRoutinesEnabled())
	provider := widget.NewEntry()
	provider.SetText(defaults.Provider)
	provider.SetPlaceHolder("(uses the global provider)")
	model := widget.NewEntry()
	model.SetText(defaults.Model)
	model.SetPlaceHolder("(uses the provider's default model)")
	timeout := widget.NewEntry()
	timeout.SetText(defaults.Timeout)
	tokenBudget := widget.NewEntry()
	tokenBudget.SetText(intPtrText(defaults.TokenBudget))
	maxTurns := widget.NewEntry()
	maxTurns.SetText(intPtrText(defaults.MaxTurns))
	maxToolCalls := widget.NewEntry()
	maxToolCalls.SetText(intPtrText(defaults.MaxToolCalls))

	items := []*widget.FormItem{
		widget.NewFormItem("", enabled),
		widget.NewFormItem("Provider", provider),
		widget.NewFormItem("Model", model),
		widget.NewFormItem("Timeout", timeout),
		widget.NewFormItem("Token budget", tokenBudget),
		widget.NewFormItem("Max turns", maxTurns),
		widget.NewFormItem("Max tool calls", maxToolCalls),
	}

	dialog.ShowForm("Routine defaults", "Save", "Cancel", items, func(ok bool) {
		if !ok {
			return
		}
		updated := config.RoutineDefaults{
			Provider: strings.TrimSpace(provider.Text),
			Model:    strings.TrimSpace(model.Text),
			Timeout:  strings.TrimSpace(timeout.Text),
		}
		budgets := []struct {
			label string
			text  string
			dest  **int
		}{
			{"token budget", tokenBudget.Text, &updated.TokenBudget},
			{"max turns", maxTurns.Text, &updated.MaxTurns},
			{"max tool calls", maxToolCalls.Text, &updated.MaxToolCalls},
		}
		for _, b := range budgets {
			value, err := parseOptionalInt(b.text)
			if err != nil {
				dialog.ShowError(fmt.Errorf("%s must be a number", b.label), g.popup.window)
				return
			}
			*b.dest = value
		}
		if err := g.cfg.SetRoutinesEnabled(enabled.Checked); err != nil {
			dialog.ShowError(err, g.popup.window)
			return
		}
		if err := g.cfg.SetRoutineDefaults(updated); err != nil {
			dialog.ShowError(err, g.popup.window)
			return
		}
	}, g.popup.window)
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
