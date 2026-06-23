package gui

import (
	"bytes"
	"context"
	"encoding/json"
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
	// routineCardTitleMax bounds the list-card title so a long name cannot collide
	// with the model shown on the right of the same row (canvas.Text neither wraps
	// nor ellipsizes on its own).
	routineCardTitleMax = 40
	// routineRefreshInterval is how often the Routine tab polls for runs produced
	// out-of-process (the daemon or the CLI) while it is visible.
	routineRefreshInterval = 4 * time.Second
	// routineDaemonOffNotice is shown when scheduled routines exist but the
	// scheduler daemon is not running, so the user knows why they will not fire.
	routineDaemonOffNotice = "Scheduler is not running, so scheduled routines won't fire. Start it with `agent daemon install` (once) or `agent daemon start`."

	// Preferred widths for the routine create/edit form and the routine defaults
	// form. The scroll width is the inner field column; the dialog width is the
	// overall popup. Both are capped to the window by routineDialogWidth.
	routineFormScrollWidth     = 560
	routineFormDialogWidth     = 600
	routineDefaultsScrollWidth = 480
	routineDefaultsDialogWidth = 520

	// routineAdvancedSectionTitle labels the collapsible section that hides the
	// rarely-changed routine fields (provider, model, budgets, deny rules, tools).
	routineAdvancedSectionTitle = "Advanced"
	// routineSummaryDetailsTitle labels the collapsible run-metadata block in a
	// summary log (ID, schedule, timings, tokens, …), collapsed by default so the
	// run's output is what the reader sees first.
	routineSummaryDetailsTitle = "Details"
	// routineSummaryMetaColumns is the column count for the run-metadata grid in a
	// summary log; two columns roughly halve the height of the property list.
	routineSummaryMetaColumns = 2

	// routineDialogChromeReserve is the vertical space (title bar, paddings, and a
	// margin from the window edges) held back from the window height when sizing
	// the scrollable field area, so the fitted dialog does not touch the edges.
	routineDialogChromeReserve = 120
	// routineMinScrollHeight is the floor for the scrollable field area so it never
	// collapses to an unusable sliver on a very short window.
	routineMinScrollHeight = 120
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
	daemonRunning := g.routineService.DaemonRunning()
	g.lastDaemonRunning = daemonRunning
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
	if routinesHaveSchedule(views) && !daemonRunning {
		notice = routineDaemonOffNotice
	}
	g.popup.setRoutines(views, "", g.showRoutineDetail, notice)
}

// maybeRefreshRoutines reloads the Routine list when it is the visible tab and
// something changed since the last load — either the routine files (a run was
// recorded or a routine edited) or the daemon's running state (so the
// scheduler-off banner appears/disappears promptly). It rebuilds only on change,
// so an idle tab does not flicker.
func (g *App) maybeRefreshRoutines() {
	if !g.state.isVisible || g.state.mode != guiModeRoutine {
		return
	}
	if latestRoutineMod().Equal(g.lastRoutineMod) && g.routineService.DaemonRunning() == g.lastDaemonRunning {
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

	title := canvas.NewText(truncateRunes(routineTitle(r), routineCardTitleMax), palette.primaryText)
	title.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	title.TextSize = theme.TextSize() - 1

	// The status dot already conveys active/inactive/error, so the row drops the
	// redundant status word. The concrete model that will run (resolved, not a bare
	// "(default)") sits in the top-right corner, level with the title.
	model := canvas.NewText(orDefaultText(view.ResolvedModel), palette.secondaryText)
	model.TextSize = theme.TextSize() - 2
	titleLeft := container.NewHBox(vCenter(newStatusDot(routineStatusColor(view.Status))), vCenter(title))
	titleRow := container.NewBorder(nil, nil, titleLeft, vCenter(model), nil)

	// Schedule, last and next runs share one line: they are all "when" information.
	times := canvas.NewText(view.Frequency+metaSeparator+"Last: "+routineTimeText(view.Run.LastRunAt)+metaSeparator+"Next: "+routineTimeText(view.Run.NextRunAt), palette.secondaryText)
	times.TextSize = theme.TextSize() - 2

	prompt := widget.NewLabel(r.PromptPreview(100))
	prompt.Wrapping = fyne.TextWrapWord

	body := container.NewVBox(titleRow, times, brandSeparator(), prompt)
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

	// Dense metadata, mirroring the card: the status dot conveys status, so it is
	// not repeated. Line 1 is the schedule and the resolved (concrete) model that
	// will run, followed only by settings that override a default — anything left
	// at its default is omitted rather than printed as "(default)". Line 2 is the
	// last and next runs. Any last error is shown below in red.
	config := []string{view.Frequency, orDefaultText(view.ResolvedProvider) + " / " + orDefaultText(view.ResolvedModel)}
	if t := strings.TrimSpace(r.Timeout); t != "" {
		config = append(config, "timeout "+t)
	}
	if r.TokenBudget != nil {
		config = append(config, "tokens "+routineBudgetText(r.TokenBudget))
	}
	if r.Tools != nil {
		config = append(config, "tools: "+routineToolPolicyText(r.Tools))
	}
	lastRun := "Last: " + routineTimeText(view.Run.LastRunAt)
	if view.HasRun {
		lastRun += " (" + view.Run.LastStatus + ")"
	}
	metaLabel := widget.NewLabel(strings.Join(config, metaSeparator) + "\n" + lastRun + metaSeparator + "Next: " + routineTimeText(view.Run.NextRunAt))
	metaLabel.Wrapping = fyne.TextWrapWord

	objects := []fyne.CanvasObject{header, metaLabel}
	if e := strings.TrimSpace(view.Run.LastError); e != "" {
		errLabel := widget.NewLabel("Error: " + e)
		errLabel.Importance = widget.DangerImportance
		errLabel.Wrapping = fyne.TextWrapWord
		objects = append(objects, errLabel)
	}

	// The prompt is set apart in its own framed, padded box so it reads as the
	// routine's content rather than more metadata; the label above is clearly a
	// heading, not the first line of the prompt.
	promptBody := widget.NewLabel(r.Prompt)
	promptBody.Wrapping = fyne.TextWrapWord
	promptBox := borderedBox(container.NewPadded(promptBody), palette.borderBright)

	objects = append(objects,
		brandSeparator(),
		brandSectionLabel("PROMPT"),
		promptBox,
		brandSeparator(),
		brandSectionLabel("RUN LOGS"),
	)
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
	// The two routine logs render differently by intent. The .md summary leads with
	// the routine name as its title, collapses the run metadata (and the source
	// file) into a dense two-column "Details" block, and shows the run output in a
	// highlighted box. The .jsonl transcript is machine data: its filename is the
	// title and the body is plain monospace pretty-printed JSON — rendering it as
	// markdown would mangle its punctuation (e.g. "~" in paths -> strikethrough).
	if ref.IsResult {
		g.popup.presentRoutineDetail(g.routineSummaryContent(data, ref.Name))
		return
	}
	label := widget.NewLabelWithStyle(routineLogText(data), fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	label.Wrapping = fyne.TextWrapWord
	g.popup.presentRoutineDetail(container.NewVBox(
		widget.NewLabelWithStyle(ref.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true, Monospace: true}),
		brandSeparator(),
		label,
	))
}

// routineSummaryContent renders a routine summary (.md). The routine name is the
// title (we already know it's a routine, so the stored "Routine:" prefix is
// dropped); the run metadata and the source file collapse into a dense two-column
// "Details" block (closed by default); and the run output is set apart in a
// highlighted box rather than under an "Output" heading. If the summary does not
// match the expected shape, the whole document is rendered as markdown unchanged.
func (g *App) routineSummaryContent(data, fileName string) fyne.CanvasObject {
	title, pairs, output, errText, ok := parseRoutineSummary(data)
	if !ok {
		md := widget.NewRichTextFromMarkdown(data)
		md.Wrapping = fyne.TextWrapWord
		return md
	}

	palette := currentBrandPalette()
	name := strings.TrimSpace(strings.TrimPrefix(title, "Routine: "))

	titleText := canvas.NewText(truncateRunes(name, routineHeaderTitleMax), palette.primaryText)
	titleText.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	titleText.TextSize = theme.TextSize() + 4

	cells := make([]fyne.CanvasObject, 0, len(pairs))
	for _, pair := range pairs {
		cell := canvas.NewText(pair, palette.secondaryText)
		cell.TextSize = theme.TextSize() - 1
		cells = append(cells, cell)
	}
	fileLine := canvas.NewText("File: "+fileName, palette.secondaryText)
	fileLine.TextSize = theme.TextSize() - 1
	detailsBody := container.NewVBox(container.NewGridWithColumns(routineSummaryMetaColumns, cells...), fileLine)
	// Expanding/collapsing changes the content height; the detail popup wraps this
	// in a fixed-size scroll, so refresh it to re-measure and re-lay-out (otherwise
	// the revealed grid is not given its rows and the cells overlap).
	details, _ := newRoutineCollapsible(routineSummaryDetailsTitle, detailsBody, func() {
		if d := g.popup.routineDetail; d != nil {
			d.Refresh()
		}
	})

	objects := []fyne.CanvasObject{titleText, brandSeparator(), details}

	if errText != "" {
		errLabel := widget.NewLabel(errText)
		errLabel.Wrapping = fyne.TextWrapWord
		errLabel.Importance = widget.DangerImportance
		objects = append(objects, borderedBox(container.NewPadded(errLabel), palette.warning))
	}

	if strings.TrimSpace(output) == "" {
		output = "(no output)"
	}
	outBody := widget.NewRichTextFromMarkdown(output)
	outBody.Wrapping = fyne.TextWrapWord
	objects = append(objects, borderedBox(container.NewPadded(outBody), palette.borderBright))

	return container.NewVBox(objects...)
}

// parseRoutineSummary splits a routine summary (.md) into its title (the text of
// the leading "# " heading), its run-metadata bullet items (each "Key: value",
// with the "- " stripped), the run output, and the optional error text. Section
// boundaries are matched as whole "## Heading" lines (not substrings), so a "##"
// occurrence inside a value or an error message is not mistaken for a heading.
// The output is everything after the "## Output" heading line — kept whole, so
// "##" headings inside the model's own output are preserved. The error is the
// body of a "## Error" section when present. ok is false when the document lacks
// a title or any metadata bullet, in which case the caller should fall back to
// rendering the whole document as markdown. CRLF line endings are tolerated.
func parseRoutineSummary(data string) (title string, pairs []string, output, errText string, ok bool) {
	lines := strings.Split(data, "\n")
	heading := func(raw string) (name string, isHeading bool) {
		raw = strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if strings.HasPrefix(raw, "## ") {
			return strings.TrimSpace(strings.TrimPrefix(raw, "## ")), true
		}
		return "", false
	}
	errIdx, outIdx := -1, -1
	for i, raw := range lines {
		switch name, isHeading := heading(raw); {
		case isHeading && name == "Error" && errIdx == -1:
			errIdx = i
		case isHeading && name == "Output" && outIdx == -1:
			outIdx = i
		}
	}
	// Title and metadata bullets live in the head — before the first section.
	headEnd := len(lines)
	for _, i := range []int{errIdx, outIdx} {
		if i >= 0 && i < headEnd {
			headEnd = i
		}
	}
	for _, raw := range lines[:headEnd] {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		switch {
		case strings.HasPrefix(line, "# ") && title == "":
			title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
		case strings.HasPrefix(line, "- "):
			pairs = append(pairs, strings.TrimSpace(strings.TrimPrefix(line, "- ")))
		}
	}
	joinLines := func(ls []string) string {
		out := make([]string, len(ls))
		for i, l := range ls {
			out[i] = strings.TrimRight(l, "\r")
		}
		return strings.TrimSpace(strings.Join(out, "\n"))
	}
	if outIdx >= 0 {
		output = joinLines(lines[outIdx+1:])
	}
	if errIdx >= 0 {
		end := len(lines)
		if outIdx > errIdx {
			end = outIdx
		}
		errText = joinLines(lines[errIdx+1 : end])
	}
	return title, pairs, output, errText, title != "" && len(pairs) > 0
}

// routineLogText prepares a routine log for plain-text display. A transcript is
// JSONL (one JSON object per line); each line is pretty-printed and the events
// are separated by a blank line so the log reads cleanly. Content that is not
// valid JSONL (e.g. a markdown summary) is returned unchanged.
func routineLogText(data string) string {
	lines := strings.Split(strings.TrimRight(data, "\n"), "\n")
	pretty := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(line), "", "  "); err != nil {
			return data // not JSONL; show as-is
		}
		pretty = append(pretty, buf.String())
	}
	if len(pretty) == 0 {
		return data
	}
	return strings.Join(pretty, "\n\n")
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
	var refit func() // re-hugs the dialog to its content; assigned once the layout exists

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
	// Labeled by the "Enabled" form-row label, so the checkbox itself has no text.
	enabled := widget.NewCheck("", nil)
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

	errorLabel := newRoutineFormError()
	// showError reveals the (otherwise hidden) error label and re-fits the dialog
	// so a long message grows the popup instead of crowding the scrolled fields.
	showError := func(msg string) {
		setRoutineFormError(errorLabel, msg)
		if refit != nil {
			refit()
		}
	}

	// Show only the essentials by default; everything else lives in a collapsed
	// "Advanced" section so the common case (name + prompt + schedule) stays simple.
	basicForm := container.New(layout.NewFormLayout(),
		formFieldLabel("Name"), themedFormField(name),
		formFieldLabel("Enabled"), enabled,
		formFieldLabel("Prompt"), themedFormField(prompt),
		formFieldLabel("Cron"), themedFormField(cron),
		widget.NewLabel(""), cronHint,
	)
	advancedForm := container.New(layout.NewFormLayout(),
		formFieldLabel("Provider"), themedFormField(provider),
		formFieldLabel("Model"), themedFormField(model),
		formFieldLabel("Timeout"), themedFormField(timeout),
		formFieldLabel("Token budget"), themedFormField(tokenBudget),
		formFieldLabel("Max turns"), themedFormField(maxTurns),
		formFieldLabel("Max tool calls"), themedFormField(maxToolCalls),
		formFieldLabel("Deny rules"), themedFormField(deny),
	)
	advanced, expandAdvanced := newRoutineCollapsible(
		routineAdvancedSectionTitle,
		container.NewVBox(advancedForm, toolsField),
		func() {
			if refit != nil {
				refit()
			}
		},
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
				// These fields live in the Advanced section; reveal it so the
				// error does not point at a control the user cannot see.
				expandAdvanced()
				showError(b.label + " " + err.Error() + ".")
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
			showError(saveErr.Error())
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
	fields := container.NewVBox(basicForm, advanced)
	bottom := container.NewVBox(errorLabel, footer)
	scroll := container.NewVScroll(fields)
	title := "New routine"
	if isEdit {
		title = "Edit routine"
	}
	dlg = dialog.NewCustomWithoutButtons(title, container.NewBorder(nil, bottom, nil, nil, scroll), win)
	dlg.SetOnClosed(g.FocusInput)
	refit = func() {
		fitRoutineFormDialog(dlg, win, scroll, fields, bottom, routineFormScrollWidth, routineFormDialogWidth)
	}
	refit()
	dlg.Show()
	// Re-fit once shown: wrapping content (the cron hint) only reports its true
	// height after it has been laid out, so the pre-show estimate understates the
	// field height and would clip the last field. Re-measuring now hugs the footer
	// to the fields with no wasted gap.
	refit()
}

// showRoutineDefaultsForm edits the defaults applied to routines that do not set
// their own values, plus the global routines on/off toggle. Reached from
// Settings → "Routine defaults…".
func (g *App) showRoutineDefaultsForm() {
	win := g.popup.window
	defaults := g.cfg.GetRoutineDefaults()
	var dlg dialog.Dialog
	var refit func() // re-hugs the dialog to its content; assigned once the layout exists

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

	errorLabel := newRoutineFormError()
	// showError reveals the (otherwise hidden) error label and re-fits the dialog
	// so a long message grows the popup instead of crowding the scrolled fields.
	showError := func(msg string) {
		setRoutineFormError(errorLabel, msg)
		if refit != nil {
			refit()
		}
	}

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
				showError(b.label + " " + err.Error() + ".")
				return
			}
			*b.dest = value
		}
		if err := g.cfg.SetRoutinesEnabled(enabled.Checked); err != nil {
			showError(err.Error())
			return
		}
		if err := g.cfg.SetRoutineDefaults(updated); err != nil {
			showError(err.Error())
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
	dlg = dialog.NewCustomWithoutButtons("Routine defaults", container.NewBorder(nil, bottom, nil, nil, scroll), win)
	refit = func() {
		fitRoutineFormDialog(dlg, win, scroll, fields, bottom, routineDefaultsScrollWidth, routineDefaultsDialogWidth)
	}
	refit()
	dlg.Show()
	// See showRoutineForm: re-fit after layout so the footer hugs the fields.
	refit()
}

func (p *popupWindow) presentRoutineDetail(content fyne.CanvasObject) {
	p.dismissRoutineDetail()
	size := historyDetailPopupSize(p.window.Canvas().Size())
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(size.Width, max(120, size.Height-historyDetailFooterHeight)))
	closeButton := widget.NewButton("Close", func() { p.dismissRoutineDetail() })
	detail := borderedBox(container.NewBorder(nil, closeButton, nil, nil, scroll), currentBrandPalette().border)
	// Use a modal popup so its backdrop dims and tracks the whole canvas, including
	// on window resize: the modal's backdrop is resized to the full canvas on every
	// layout. A non-modal popup is sized once at creation and only ever shrinks to
	// fit, so it cannot grow to fill a resized window and the dim stays a fixed box.
	pop := widget.NewModalPopUp(detail, p.window.Canvas())
	p.routineDetail = pop
	p.window.Canvas().Unfocus()
	pop.Show()
	pop.Resize(size)
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

// newRoutineCollapsible wraps content in a titled section that starts closed, so
// secondary detail does not take space until asked for. Tapping the header toggles
// the content's visibility (a VBox skips hidden children, so a closed section adds
// only the header's height) and calls onToggle (may be nil), used e.g. to re-fit a
// dialog so it grows or shrinks to the new height. The returned expand func opens
// the section programmatically (idempotent).
func newRoutineCollapsible(title string, content fyne.CanvasObject, onToggle func()) (section fyne.CanvasObject, expand func()) {
	content.Hide()
	var toggle *widget.Button
	setOpen := func(open bool) {
		if open == content.Visible() {
			return
		}
		if open {
			content.Show()
			toggle.SetIcon(theme.MenuDropDownIcon())
		} else {
			content.Hide()
			toggle.SetIcon(theme.MenuExpandIcon())
		}
		if onToggle != nil {
			onToggle()
		}
	}
	toggle = widget.NewButtonWithIcon(title, theme.MenuExpandIcon(), func() {
		setOpen(!content.Visible())
	})
	toggle.Importance = widget.LowImportance
	toggle.Alignment = widget.ButtonAlignLeading
	return container.NewVBox(toggle, content), func() { setOpen(true) }
}

// newRoutineFormError builds the validation/error label used in the routine
// forms. It starts hidden so the pinned footer collapses to just the button row
// (a VBox skips non-visible children); setRoutineFormError reveals it only when
// there is a message, so no blank line is reserved in the common case.
func newRoutineFormError() *widget.Label {
	label := widget.NewLabel("")
	label.Wrapping = fyne.TextWrapWord
	label.Importance = widget.DangerImportance
	label.Hide()
	return label
}

// setRoutineFormError shows msg in the form's error label, or hides the label
// entirely when msg is empty so the footer takes no more height than the buttons
// require.
func setRoutineFormError(label *widget.Label, msg string) {
	if msg == "" {
		label.SetText("")
		label.Hide()
		return
	}
	label.SetText(msg)
	label.Show()
}

// fitRoutineFormDialog sizes the scrolling field area to the fields' natural
// height (capped to the window) and hugs the dialog around it, so the pinned
// footer sits directly below the fields with no wasted gap. It resizes to the
// dialog's live minimum height (rather than zero) because Fyne's Resize is a
// no-op when the size is unchanged; the concrete height forces a relayout. Call
// it again after Show() because wrapping content (e.g. the cron hint) only
// reports its true height once it has been laid out at a known width, so the
// pre-show measurement understates the field height and would clip the last
// field.
func fitRoutineFormDialog(dlg dialog.Dialog, win fyne.Window, scroll *container.Scroll, fields, bottom fyne.CanvasObject, scrollWidth, dialogWidth float32) {
	scroll.SetMinSize(fyne.NewSize(routineDialogWidth(win, scrollWidth), routineScrollHeight(win, fields, bottom)))
	dlg.Resize(fyne.NewSize(routineDialogWidth(win, dialogWidth), dlg.MinSize().Height))
}

// routineScrollHeight sizes the scrolling field area to the fields' natural
// height so the dialog hugs the content (no wasted space above the pinned
// bottom bar), capped at the window height (less the bottom bar) so a tall form
// scrolls instead of overflowing on a small window. bottom is the pinned
// error-plus-buttons container, not just the button footer.
func routineScrollHeight(win fyne.Window, fields, bottom fyne.CanvasObject) float32 {
	available := win.Canvas().Size().Height - routineDialogChromeReserve - bottom.MinSize().Height
	if available < routineMinScrollHeight {
		available = routineMinScrollHeight
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
