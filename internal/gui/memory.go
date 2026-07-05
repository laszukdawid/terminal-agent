package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/laszukdawid/terminal-agent/internal/agent"
	appservice "github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/taskmemory"
)

const (
	// memoryRecentLimit is how many newest records the Task Memory tab lists when
	// no search query is entered.
	memoryRecentLimit = 50
	// memorySearchLimit is how many matches a Task Memory search shows; larger
	// than the in-task retrieval limit because the tab is for human browsing.
	memorySearchLimit = 20
	// memoryQueryTimeout bounds every store operation issued by the GUI, matching
	// the one-second budget the CLI uses for task memory searches.
	memoryQueryTimeout = time.Second

	memorySearchPlaceholder = "Search task memories"
	memorySearchButtonText  = "SEARCH"
	memoryClearButtonText   = "CLEAR"
	// memoryIndexingDisabledNotice is shown above the list while `task_memory` is
	// off in config, so the user knows why new task runs stop appearing here.
	memoryIndexingDisabledNotice = "Task memory indexing is disabled (task_memory in config), so new task runs are not added."

	memoryCardGap = 6
	// memoryCommandPrompt marks the executed command on a memory card, matching
	// the "$" output markers of rendered responses.
	memoryCommandPrompt = "$"
	// memoryPreviewMaxBodyLines bounds the output/answer/error block of a card,
	// which sits under the request and command lines.
	memoryPreviewMaxBodyLines = 4
	// memoryRunMaxCommands bounds how many step commands a browse card lists for
	// one run; the run detail popup always lists every record.
	memoryRunMaxCommands = 3
	// memoryRunTitleKind heads a browse card that merges a whole run's records.
	memoryRunTitleKind = "TASK"
)

// loadTaskMemories populates the Task Memory tab: matches for the current
// search query, or the newest records when the query is empty.
func (g *App) loadTaskMemories() {
	query := ""
	if g.popup.memorySearch != nil {
		query = strings.TrimSpace(g.popup.memorySearch.Text)
	}
	notice := ""
	if g.cfg != nil && !g.cfg.GetTaskMemory() {
		notice = memoryIndexingDisabledNotice
	}
	records, stats, err := fetchTaskMemories(query)
	if err != nil {
		g.popup.setTaskMemories(nil, taskmemory.Stats{}, query, "Task memory unavailable: "+err.Error(), notice)
		return
	}
	g.popup.setTaskMemories(records, stats, query, "", notice)
}

// fetchTaskMemories opens the task memory database for a single read, mirroring
// the CLI which opens per command; WAL mode keeps that safe alongside a running
// task that is indexing new records.
func fetchTaskMemories(query string) ([]taskmemory.Record, taskmemory.Stats, error) {
	store, err := taskmemory.Open(appservice.TaskMemoryPath())
	if err != nil {
		return nil, taskmemory.Stats{}, err
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), memoryQueryTimeout)
	defer cancel()
	stats, err := store.Stats(ctx)
	if err != nil {
		return nil, taskmemory.Stats{}, err
	}
	if query == "" {
		records, err := store.Recent(ctx, memoryRecentLimit)
		return records, stats, err
	}
	env := taskmemory.CurrentEnvironment("")
	matches, err := store.Search(ctx, taskmemory.Query{Text: query, OS: env.OS, Arch: env.Arch, Shell: env.Shell, Limit: memorySearchLimit})
	if err != nil {
		return nil, stats, err
	}
	records := make([]taskmemory.Record, 0, len(matches))
	for _, match := range matches {
		records = append(records, match.Record)
	}
	return records, stats, nil
}

// confirmClearTaskMemories asks before wiping the index. Session logs under the
// data directory are unaffected and remain the authoritative task history.
func (g *App) confirmClearTaskMemories() {
	dialog.ShowConfirm("Clear task memory", "Delete all indexed task memories? Session logs are kept.", func(ok bool) {
		if !ok {
			return
		}
		if err := clearTaskMemories(); err != nil {
			dialog.ShowError(err, g.popup.window)
			return
		}
		g.loadTaskMemories()
	}, g.popup.window)
}

func clearTaskMemories() error {
	store, err := taskmemory.Open(appservice.TaskMemoryPath())
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), memoryQueryTimeout)
	defer cancel()
	return store.Clear(ctx)
}

// buildMemorySection creates the Task Memory tab: a search row over a scrolling
// list of indexed records, with a record-count meta line and a CLEAR action.
func (p *popupWindow) buildMemorySection() *fyne.Container {
	palette := currentBrandPalette()

	search := widget.NewEntry()
	search.SetPlaceHolder(memorySearchPlaceholder)
	search.OnSubmitted = func(string) { p.submitMemorySearch() }
	p.memorySearch = search
	searchButton := newCommandButton(memorySearchButtonText, iconPathSearch, p.submitMemorySearch)

	clearButton := newCommandButton(memoryClearButtonText, iconPathTrash, func() {
		if p.onMemoryClear != nil {
			p.onMemoryClear()
		}
	})

	p.memoryStats = newElidedText("", palette.secondaryText)
	p.memoryStats.SetTextSize(theme.TextSize() - 2)

	p.memoryBody = container.NewVBox()

	header := container.NewBorder(nil, nil, brandSectionLabel(sectionMemory), clearButton, nil)
	searchRow := container.NewBorder(nil, nil, nil, vCenter(searchButton), withNativeCursor(search))
	statsRow := container.NewVBox(brandSeparator(), p.memoryStats)

	return container.NewBorder(
		container.NewVBox(header, searchRow),
		statsRow,
		nil, nil,
		borderedBox(container.NewVScroll(p.memoryBody), palette.border),
	)
}

func (p *popupWindow) submitMemorySearch() {
	if p.onMemorySearch != nil {
		p.onMemorySearch()
	}
}

func (p *popupWindow) setTaskMemories(records []taskmemory.Record, stats taskmemory.Stats, query, errorText, notice string) {
	if p.memoryBody == nil {
		return
	}
	p.memoryBody.Objects = nil
	if notice != "" {
		noticeLabel := widget.NewLabel(notice)
		noticeLabel.Wrapping = fyne.TextWrapWord
		noticeLabel.Importance = widget.WarningImportance
		p.memoryBody.Add(noticeLabel)
	}
	shown := len(records)
	switch {
	case errorText != "":
		label := widget.NewLabel(errorText)
		label.Wrapping = fyne.TextWrapWord
		p.memoryBody.Add(label)
	case len(records) == 0:
		p.memoryBody.Add(memoryEmptyState(query))
	case query == "":
		// Browsing shows one card per task run; the run's records are merged so
		// a single task does not repeat as request/step/completion cards.
		groups := groupMemoryRecords(records)
		shown = len(groups)
		for _, group := range groups {
			card := newTappableHistoryCard(memoryRunCardContent(group), func() {
				p.showMemoryRunDetail(group)
			})
			p.memoryBody.Add(container.New(layout.NewCustomPaddedLayout(0, memoryCardGap, 0, 0), card))
		}
	default:
		// Search results stay per-record: a match is a specific record and rank.
		for _, rec := range records {
			card := newTappableHistoryCard(memoryCardContent(rec), func() {
				p.showMemoryDetail(rec)
			})
			p.memoryBody.Add(container.New(layout.NewCustomPaddedLayout(0, memoryCardGap, 0, 0), card))
		}
	}
	p.setMemoryStats(memoryStatsText(stats, shown, query))
	p.memoryBody.Refresh()
}

// memoryRunGroup is one task run's records, newest first as fetched, grouped
// for the browse view.
type memoryRunGroup struct {
	runID   string
	records []taskmemory.Record
}

// groupMemoryRecords groups records by source run, keeping the newest-first
// input order (a run sorts where its newest record appears). Records without
// any grouping identity each form their own group.
func groupMemoryRecords(records []taskmemory.Record) []memoryRunGroup {
	groups := make([]memoryRunGroup, 0, len(records))
	index := map[string]int{}
	for i, rec := range records {
		key := firstNonEmptyString(rec.SourceRunID, rec.ID, fmt.Sprintf("record-%d", i))
		at, ok := index[key]
		if !ok {
			at = len(groups)
			index[key] = at
			groups = append(groups, memoryRunGroup{runID: rec.SourceRunID})
		}
		groups[at].records = append(groups[at].records, rec)
	}
	return groups
}

// memoryRunCardContent merges one run's records into a single browse card: the
// request once, the commands its steps ran, and the final answer or, for a run
// that never completed, its last error.
func memoryRunCardContent(group memoryRunGroup) fyne.CanvasObject {
	palette := currentBrandPalette()
	title := newElidedText(memoryRunTitle(group), palette.primaryText)
	title.SetTextStyle(fyne.TextStyle{Bold: true, Monospace: true})
	title.SetTextSize(theme.TextSize() - 1)

	meta := newElidedText(memoryRunMeta(group), palette.secondaryText)
	meta.SetTextSize(theme.TextSize() - 2)

	rows := []fyne.CanvasObject{title, meta, brandSeparator()}
	truncated := false

	if request, cut := historyPreview(memoryRunRequest(group), ""); request != "" {
		truncated = truncated || cut
		rows = append(rows, historyQueryLine(request))
	}
	commands := memoryRunCommands(group)
	if len(commands) > memoryRunMaxCommands {
		commands = commands[:memoryRunMaxCommands]
		truncated = true
	}
	for _, command := range commands {
		text, cut := historyPreview(command, "")
		truncated = truncated || cut
		rows = append(rows, memoryCommandLine(text))
	}
	if outcome, isError := memoryRunOutcome(group); outcome != "" {
		body, cut := memoryPlainPreview(outcome)
		truncated = truncated || cut
		label := widget.NewLabel(body)
		label.Wrapping = fyne.TextWrapWord
		if isError {
			label.Importance = widget.DangerImportance
		}
		rows = append(rows, label)
	}
	if len(rows) == 3 && len(group.records) > 0 {
		text, cut := historyPreview(memoryPreviewSource(group.records[0]), "No indexed text.")
		truncated = truncated || cut
		label := widget.NewLabel(text)
		label.Wrapping = fyne.TextWrapWord
		rows = append(rows, label)
	}

	card := borderedBox(container.NewVBox(rows...), palette.border)
	if truncated {
		return withHistoryTruncationCorner(card)
	}
	return card
}

func memoryRunTitle(group memoryRunGroup) string {
	when := "unknown time"
	if len(group.records) > 0 && !group.records[0].CreatedAt.IsZero() {
		when = group.records[0].CreatedAt.Local().Format("2006-01-02 15:04")
	}
	return memoryRunTitleKind + "  " + when
}

// memoryRunMeta summarizes the run: the tools its steps used, the environment,
// the working directory, and how many records the run indexed.
func memoryRunMeta(group memoryRunGroup) string {
	parts := make([]string, 0, 4)
	if tools := memoryRunTools(group); len(tools) > 0 {
		parts = append(parts, strings.Join(tools, ", "))
	}
	if len(group.records) > 0 {
		rec := group.records[0]
		if rec.OS != "" {
			env := rec.OS
			if rec.Arch != "" {
				env += "/" + rec.Arch
			}
			parts = append(parts, env)
		}
		if rec.CWD != "" {
			parts = append(parts, displayCwd(rec.CWD))
		}
	}
	if n := len(group.records); n == 1 {
		parts = append(parts, "1 record")
	} else {
		parts = append(parts, fmt.Sprintf("%d records", n))
	}
	return strings.Join(parts, metaSeparator)
}

// memoryRunTools lists the distinct tools the run's steps used, in execution
// order. The final_answer pseudo-tool is delivery, not a learned action.
func memoryRunTools(group memoryRunGroup) []string {
	seen := map[string]bool{}
	var out []string
	for i := len(group.records) - 1; i >= 0; i-- {
		name := group.records[i].ToolName
		if name == "" || name == agent.ToolNameFinalAnswer || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func memoryRunRequest(group memoryRunGroup) string {
	for _, rec := range group.records {
		if request := parseMemoryFields(rec.SearchableText).Request; request != "" {
			return request
		}
	}
	return ""
}

// memoryRunCommands returns the distinct commands the run's tool steps ran, in
// execution order (oldest record first).
func memoryRunCommands(group memoryRunGroup) []string {
	seen := map[string]bool{}
	var out []string
	for i := len(group.records) - 1; i >= 0; i-- {
		rec := group.records[i]
		if rec.Kind != taskmemory.KindToolSuccess && rec.Kind != taskmemory.KindToolFailure {
			continue
		}
		command := memoryCommandText(rec)
		if command == "" || seen[command] {
			continue
		}
		seen[command] = true
		out = append(out, command)
	}
	return out
}

// memoryRunOutcome picks the run's result for the card body: the newest final
// answer when the run completed, otherwise its newest error, otherwise the
// newest tool output so a step-only run still shows what it learned.
func memoryRunOutcome(group memoryRunGroup) (string, bool) {
	for _, rec := range group.records {
		if rec.Kind != taskmemory.KindTaskCompletion {
			continue
		}
		fields := parseMemoryFields(rec.SearchableText)
		if text := firstNonEmptyString(rec.FinalOutput, fields.Answer, fields.Output); text != "" {
			return text, false
		}
	}
	for _, rec := range group.records {
		if rec.Kind != taskmemory.KindToolFailure {
			continue
		}
		fields := parseMemoryFields(rec.SearchableText)
		if text := firstNonEmptyString(fields.Error, rec.FinalOutput, fields.Output); text != "" {
			return text, true
		}
	}
	for _, rec := range group.records {
		if rec.Kind != taskmemory.KindToolSuccess {
			continue
		}
		fields := parseMemoryFields(rec.SearchableText)
		if text := firstNonEmptyString(rec.FinalOutput, fields.Output); text != "" {
			return text, false
		}
	}
	return "", false
}

// showMemoryRunDetail presents a whole run: the request once, then each record
// in execution order with its command, thought, and result. Per-record detail
// (including the raw indexed text) remains available from search results,
// which list records individually.
func (p *popupWindow) showMemoryRunDetail(group memoryRunGroup) {
	p.dismissMemoryDetail()
	content := container.NewVBox(
		widget.NewLabelWithStyle(memoryRunTitle(group), fyne.TextAlignLeading, fyne.TextStyle{Bold: true, Monospace: true}),
		widget.NewLabel(memoryRunDetailMeta(group)),
		brandSeparator(),
	)
	if request := memoryRunRequest(group); request != "" {
		content.Add(historyQueryLine(request))
	}
	for i := len(group.records) - 1; i >= 0; i-- {
		rec := group.records[i]
		if rec.Kind == taskmemory.KindTaskRequest {
			continue // already represented by the request line
		}
		content.Add(memoryRunDetailRecordSection(rec))
	}

	size := historyDetailPopupSize(p.window.Canvas().Size())
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(size.Width, max(120, size.Height-historyDetailFooterHeight)))
	closeButton := widget.NewButton("Close", func() { p.dismissMemoryDetail() })
	detail := borderedBox(container.NewBorder(nil, closeButton, nil, nil, scroll), currentBrandPalette().border)
	pop := widget.NewModalPopUp(detail, p.window.Canvas())
	p.memoryDetail = pop
	p.window.Canvas().Unfocus()
	pop.Show()
	pop.Resize(size)
}

func memoryRunDetailMeta(group memoryRunGroup) string {
	parts := make([]string, 0, 3)
	if meta := memoryRunMeta(group); meta != "" {
		parts = append(parts, meta)
	}
	if len(group.records) > 0 && group.records[0].Shell != "" {
		parts = append(parts, "shell: "+group.records[0].Shell)
	}
	if group.runID != "" {
		parts = append(parts, "run "+group.runID)
	}
	return strings.Join(parts, metaSeparator)
}

// memoryRunDetailRecordSection renders one record of a run: its kind and step
// as the heading, then the command, thought, and full result.
func memoryRunDetailRecordSection(rec taskmemory.Record) fyne.CanvasObject {
	heading := memoryTitle(rec)
	if rec.SourceSeq > 0 {
		heading += fmt.Sprintf("%sstep %d", metaSeparator, rec.SourceSeq)
	}
	section := container.NewVBox(
		brandSeparator(),
		widget.NewLabelWithStyle(heading, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)
	if command := memoryCommandText(rec); command != "" {
		section.Add(memoryCommandLine(command))
	}
	fields := parseMemoryFields(rec.SearchableText)
	if fields.Thought != "" {
		section.Add(memoryDetailSection("Thought", fields.Thought))
	}
	if body := memoryBodyText(rec, fields); body != "" {
		label := widget.NewLabel(body)
		label.Wrapping = fyne.TextWrapWord
		label.TextStyle = fyne.TextStyle{Monospace: true}
		if rec.Kind == taskmemory.KindToolFailure {
			label.Importance = widget.DangerImportance
		}
		section.Add(label)
	}
	return section
}

func (p *popupWindow) setMemoryStats(text string) {
	if p.memoryStats == nil || p.memoryStats.Text() == text {
		return
	}
	p.memoryStats.SetText(text)
}

// memoryStatsText is the meta line under the list: index size, how much the
// current view shows (runs when browsing, matched records when searching), and
// a note when FTS5 is unavailable (search then falls back to lexical matching).
func memoryStatsText(stats taskmemory.Stats, shown int, query string) string {
	parts := make([]string, 0, 3)
	parts = append(parts, fmt.Sprintf("%d records indexed", stats.Records))
	switch {
	case query != "":
		parts = append(parts, fmt.Sprintf("%d matches shown", shown))
	case shown == 1:
		parts = append(parts, "1 recent run shown")
	case shown > 1:
		parts = append(parts, fmt.Sprintf("%d recent runs shown", shown))
	}
	if !stats.FTSEnabled {
		parts = append(parts, "full-text search unavailable")
	}
	return strings.Join(parts, metaSeparator)
}

func memoryEmptyState(query string) fyne.CanvasObject {
	palette := currentBrandPalette()
	text := "No task memories recorded yet. Run `agent task` and its steps will be indexed here."
	if query != "" {
		text = "No task memories match this search."
	}
	label := newElidedText(text, palette.secondaryText)
	label.SetTextSize(theme.TextSize())
	return container.NewPadded(label)
}

// memoryCardContent renders a record as intent rather than as its raw index
// text: the user request behind the ">" input marker, the executed command
// behind "$", and the output, answer, or error as the body. The raw indexed
// text remains available in the detail popup.
func memoryCardContent(rec taskmemory.Record) fyne.CanvasObject {
	palette := currentBrandPalette()
	title := newElidedText(memoryTitle(rec), palette.primaryText)
	title.SetTextStyle(fyne.TextStyle{Bold: true, Monospace: true})
	title.SetTextSize(theme.TextSize() - 1)

	meta := newElidedText(memoryMeta(rec), palette.secondaryText)
	meta.SetTextSize(theme.TextSize() - 2)

	rows := []fyne.CanvasObject{title, meta, brandSeparator()}
	truncated := false

	fields := parseMemoryFields(rec.SearchableText)
	if request, cut := historyPreview(fields.Request, ""); request != "" {
		truncated = truncated || cut
		rows = append(rows, historyQueryLine(request))
	}
	if command, cut := historyPreview(memoryCommandText(rec), ""); command != "" {
		truncated = truncated || cut
		rows = append(rows, memoryCommandLine(command))
	}
	if body, cut := memoryPlainPreview(memoryBodyText(rec, fields)); body != "" {
		truncated = truncated || cut
		label := widget.NewLabel(body)
		label.Wrapping = fyne.TextWrapWord
		if rec.Kind == taskmemory.KindToolFailure {
			label.Importance = widget.DangerImportance
		}
		rows = append(rows, label)
	}
	if len(rows) == 3 {
		// Nothing structured was recognized; fall back to the raw indexed text.
		text, cut := historyPreview(memoryPreviewSource(rec), "No indexed text.")
		truncated = truncated || cut
		label := widget.NewLabel(text)
		label.Wrapping = fyne.TextWrapWord
		rows = append(rows, label)
	}

	card := borderedBox(container.NewVBox(rows...), palette.border)
	if truncated {
		return withHistoryTruncationCorner(card)
	}
	return card
}

// memoryFields are the labeled parts of a record's indexed text that the
// structured record does not carry separately (the request, thought, and
// error). taskmemory.BuildSearchableText emits "Label: value" lines whose
// values run until the next labeled line, so they are parsed back here to
// present intent instead of the raw index dump.
type memoryFields struct {
	Request string
	Thought string
	Answer  string
	Output  string
	Error   string
}

// memoryFieldLabels are the line prefixes taskmemory's record builders emit.
var memoryFieldLabels = []string{
	"Kind:", "User request:", "Thought:", "Tool:", "Input:",
	"Output:", "Error:", "Final answer:", "Raw output:", "OS:", "CWD:",
}

func parseMemoryFields(text string) memoryFields {
	values := map[string][]string{}
	current := ""
	for _, line := range strings.Split(text, "\n") {
		if label := matchMemoryFieldLabel(line); label != "" {
			current = label
			values[current] = append(values[current], strings.TrimPrefix(line, label))
			continue
		}
		if current != "" {
			values[current] = append(values[current], line)
		}
	}
	field := func(label string) string {
		return strings.TrimSpace(strings.Join(values[label], "\n"))
	}
	return memoryFields{
		Request: field("User request:"),
		Thought: field("Thought:"),
		Answer:  field("Final answer:"),
		Output:  field("Output:"),
		Error:   field("Error:"),
	}
}

func matchMemoryFieldLabel(line string) string {
	for _, label := range memoryFieldLabels {
		if strings.HasPrefix(line, label) {
			return label
		}
	}
	return ""
}

// memoryCommandText is the actionable part of a tool record: the shell command
// or python snippet itself, or the tool name with its structured input for
// other tools. A bare tool name adds nothing beyond the meta line, and the
// final_answer input duplicates the answer body, so both yield "".
func memoryCommandText(rec taskmemory.Record) string {
	if rec.ToolName == agent.ToolNameFinalAnswer {
		return ""
	}
	for _, key := range []string{"command", "code"} {
		if value, ok := rec.ToolInput[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	if len(rec.ToolInput) == 0 {
		return ""
	}
	return rec.ToolName + " " + taskmemory.ToolInputJSON(rec.ToolInput)
}

func memoryCommandLine(text string) *widget.RichText {
	return markedTextLine(memoryCommandPrompt, text, fyne.TextStyle{Monospace: true})
}

// memoryBodyText picks the content a card previews under the request and
// command lines: the error for failures, the final answer for completions,
// and the tool output for successes.
func memoryBodyText(rec taskmemory.Record, fields memoryFields) string {
	switch rec.Kind {
	case taskmemory.KindToolFailure:
		return firstNonEmptyString(fields.Error, rec.FinalOutput, fields.Output)
	case taskmemory.KindTaskCompletion:
		return firstNonEmptyString(rec.FinalOutput, fields.Answer, fields.Output)
	case taskmemory.KindToolSuccess:
		return firstNonEmptyString(rec.FinalOutput, fields.Output)
	}
	return ""
}

// memoryPlainPreview bounds plain terminal text (tool output, answers, errors)
// for a card while keeping its line structure, completing the word at the cut
// like the history previews do.
func memoryPlainPreview(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	budget := &richTextBudget{runes: historyPreviewMaxRunes, lines: memoryPreviewMaxBodyLines}
	kept, cut := budget.take(value)
	if cut {
		return kept + previewEllipsis, true
	}
	return kept, false
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// memoryTitle renders the record kind and local time, mirroring history cards.
func memoryTitle(rec taskmemory.Record) string {
	kind := strings.ToUpper(strings.ReplaceAll(rec.Kind, "_", " "))
	if kind == "" {
		kind = "MEMORY"
	}
	when := "unknown time"
	if !rec.CreatedAt.IsZero() {
		when = rec.CreatedAt.Local().Format("2006-01-02 15:04")
	}
	return kind + "  " + when
}

func memoryMeta(rec taskmemory.Record) string {
	parts := make([]string, 0, 3)
	if rec.ToolName != "" {
		parts = append(parts, rec.ToolName)
	}
	if rec.OS != "" {
		env := rec.OS
		if rec.Arch != "" {
			env += "/" + rec.Arch
		}
		parts = append(parts, env)
	}
	if rec.CWD != "" {
		parts = append(parts, displayCwd(rec.CWD))
	}
	return strings.Join(parts, metaSeparator)
}

// memoryPreviewSource is the fallback card body for records with nothing
// structured to show. Lines duplicating the card chrome are dropped: "Kind:"
// repeats the title and "OS:"/"CWD:" repeat the meta line.
// The searchable text already leads
// with a "Kind: …" line that duplicates the card title, so it is dropped.
func memoryPreviewSource(rec taskmemory.Record) string {
	lines := strings.Split(rec.SearchableText, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "Kind: ") || strings.HasPrefix(line, "OS: ") || strings.HasPrefix(line, "CWD: ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func (p *popupWindow) showMemoryDetail(rec taskmemory.Record) {
	p.dismissMemoryDetail()
	content := container.NewVBox(
		widget.NewLabelWithStyle(memoryTitle(rec), fyne.TextAlignLeading, fyne.TextStyle{Bold: true, Monospace: true}),
		widget.NewLabel(memoryDetailMeta(rec)),
		brandSeparator(),
	)
	fields := parseMemoryFields(rec.SearchableText)
	if fields.Request != "" {
		content.Add(historyQueryLine(fields.Request))
	}
	if fields.Thought != "" {
		content.Add(memoryDetailSection("Thought", fields.Thought))
	}
	if len(rec.ToolInput) > 0 {
		content.Add(memoryDetailSection("Tool input", memoryToolInputText(rec.ToolInput)))
	}
	if strings.TrimSpace(rec.FinalOutput) != "" {
		content.Add(memoryDetailSection("Output", rec.FinalOutput))
	}
	if fields.Error != "" {
		content.Add(memoryDetailSection("Error", fields.Error))
	}
	content.Add(memoryDetailSection("Indexed text", rec.SearchableText))

	size := historyDetailPopupSize(p.window.Canvas().Size())
	scroll := container.NewVScroll(content)
	scroll.SetMinSize(fyne.NewSize(size.Width, max(120, size.Height-historyDetailFooterHeight)))
	closeButton := widget.NewButton("Close", func() { p.dismissMemoryDetail() })
	detail := borderedBox(container.NewBorder(nil, closeButton, nil, nil, scroll), currentBrandPalette().border)
	pop := widget.NewModalPopUp(detail, p.window.Canvas())
	p.memoryDetail = pop
	p.window.Canvas().Unfocus()
	pop.Show()
	pop.Resize(size)
}

// memoryDetailMeta extends the card meta with shell and provenance (source run
// and step) so a record can be traced back to its session log.
func memoryDetailMeta(rec taskmemory.Record) string {
	parts := make([]string, 0, 3)
	if meta := memoryMeta(rec); meta != "" {
		parts = append(parts, meta)
	}
	if rec.Shell != "" {
		parts = append(parts, "shell: "+rec.Shell)
	}
	if rec.SourceRunID != "" {
		source := "run " + rec.SourceRunID
		if rec.SourceSeq > 0 {
			source += fmt.Sprintf(" step %d", rec.SourceSeq)
		}
		parts = append(parts, source)
	}
	return strings.Join(parts, metaSeparator)
}

func memoryToolInputText(input map[string]any) string {
	encoded, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return fmt.Sprint(input)
	}
	return string(encoded)
}

// memoryDetailSection renders record fields as monospace blocks: tool input,
// output, and indexed text are machine text, not markdown.
func memoryDetailSection(title, text string) fyne.CanvasObject {
	body := widget.NewLabel(text)
	body.Wrapping = fyne.TextWrapWord
	body.TextStyle = fyne.TextStyle{Monospace: true}
	return container.NewVBox(
		widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		body,
	)
}

func (p *popupWindow) dismissMemoryDetail() bool {
	if p.memoryDetail == nil || !p.memoryDetail.Visible() {
		return false
	}
	p.memoryDetail.Hide()
	return true
}
