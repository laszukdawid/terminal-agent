package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	internalagent "github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/daemon"
	"github.com/laszukdawid/terminal-agent/internal/routines"
	"github.com/laszukdawid/terminal-agent/internal/sessionlog"
)

// maxResultSummaries bounds how many run artifacts are retained per routine.
const maxResultSummaries = 50

// RoutineService is the app-layer facade for managing and running routines. It
// is shared by the CLI, the GUI, and the scheduling daemon so every surface uses
// one execution and persistence path.
type RoutineService interface {
	List(ctx context.Context) ([]RoutineView, error)
	Get(ctx context.Context, idOrName string) (RoutineView, error)
	// Create stores a new routine, assigning an id from the name when unset, and
	// fails if the id already exists (no silent overwrite).
	Create(ctx context.Context, r routines.Routine) (routines.Routine, error)
	// Save inserts or updates a routine by id (upsert); used for edits like
	// enable/disable.
	Save(ctx context.Context, r routines.Routine) (routines.Routine, error)
	Delete(ctx context.Context, idOrName string, purge bool) error
	Run(ctx context.Context, req RoutineRunRequest) (RoutineRunResult, error)
	Logs(ctx context.Context, idOrName string) ([]RoutineLogRef, error)
	// ReadLog returns the content of a stored run log, keeping filesystem access
	// behind the facade rather than in callers (e.g. the GUI).
	ReadLog(ctx context.Context, ref RoutineLogRef) (string, error)
	// LaunchNotice returns a one-line summary of routine runs that completed
	// since the user last saw routine activity, then advances the seen marker.
	// It returns an empty string when there is nothing to report.
	LaunchNotice(ctx context.Context) (string, error)
	// DaemonRunning reports whether the scheduler daemon is currently running.
	// Scheduled routines only fire while it is; surfaces should warn when it isn't.
	DaemonRunning() bool
}

// RoutineView merges a routine definition with its latest run status for display.
type RoutineView struct {
	Routine   routines.Routine
	Status    string // active | inactive | error
	Run       routines.RunRecord
	HasRun    bool
	Frequency string // human-readable schedule
	// ResolvedProvider/ResolvedModel are the provider and model the routine would
	// actually run with: its own values when set, otherwise the routine defaults
	// and finally the global config. Surfacing them lets the UI show the concrete
	// model instead of a bare "(default)" that hides what will execute.
	ResolvedProvider string
	ResolvedModel    string
}

// RoutineRunRequest asks the service to run a routine now.
type RoutineRunRequest struct {
	IDOrName string
	Trigger  string // routines.TriggerManual (default) or routines.TriggerScheduled
}

// RoutineRunResult is the outcome of a single routine run.
type RoutineRunResult struct {
	Routine    routines.Routine
	Output     string
	Outcome    string // routines.Outcome*
	Err        error
	Duration   time.Duration
	TokensUsed int
	SessionLog string
	ResultPath string
}

// RoutineLogRef points at a stored run artifact (transcript or summary).
type RoutineLogRef struct {
	Path     string
	Name     string
	ModTime  time.Time
	IsResult bool // true for the human-readable .md summary, false for the .jsonl transcript
}

type routineService struct {
	cfg   config.Config
	store *routines.Store
	state *routines.StateStore
}

// NewRoutineService builds a RoutineService backed by the standard routine
// stores (which honor the test directory overrides).
func NewRoutineService(cfg config.Config) RoutineService {
	return &routineService{
		cfg:   cfg,
		store: routines.DefaultStore(),
		state: routines.DefaultStateStore(),
	}
}

func (s *routineService) DaemonRunning() bool {
	status, err := daemon.CurrentStatus()
	return err == nil && status.Running
}

func (s *routineService) List(ctx context.Context) ([]RoutineView, error) {
	defs, err := s.store.List()
	if err != nil {
		return nil, err
	}
	states, err := s.state.All()
	if err != nil {
		return nil, err
	}
	views := make([]RoutineView, 0, len(defs))
	for _, r := range defs {
		rec, has := states[r.ID]
		eff := s.resolve(r)
		views = append(views, RoutineView{
			Routine:          r,
			Status:           routines.DisplayStatus(r.Enabled, rec, has),
			Run:              rec,
			HasRun:           has,
			Frequency:        describeSchedule(r.Schedule),
			ResolvedProvider: eff.Provider,
			ResolvedModel:    eff.Model,
		})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Routine.ID < views[j].Routine.ID })
	return views, nil
}

func (s *routineService) Get(ctx context.Context, idOrName string) (RoutineView, error) {
	r, err := s.store.Get(idOrName)
	if err != nil {
		return RoutineView{}, err
	}
	rec, has, err := s.state.Get(r.ID)
	if err != nil {
		return RoutineView{}, err
	}
	eff := s.resolve(r)
	return RoutineView{
		Routine:          r,
		Status:           routines.DisplayStatus(r.Enabled, rec, has),
		Run:              rec,
		HasRun:           has,
		Frequency:        describeSchedule(r.Schedule),
		ResolvedProvider: eff.Provider,
		ResolvedModel:    eff.Model,
	}, nil
}

func (s *routineService) Create(ctx context.Context, r routines.Routine) (routines.Routine, error) {
	if strings.TrimSpace(r.ID) == "" {
		r.ID = routines.UniqueID(r.Name, s.store.IDTaken)
	}
	if err := s.store.Add(r); err != nil {
		return routines.Routine{}, err
	}
	return s.store.Get(r.ID)
}

func (s *routineService) Save(ctx context.Context, r routines.Routine) (routines.Routine, error) {
	if strings.TrimSpace(r.ID) == "" {
		r.ID = routines.UniqueID(r.Name, s.store.IDTaken)
	}
	if err := s.store.Upsert(r); err != nil {
		return routines.Routine{}, err
	}
	return s.store.Get(r.ID)
}

func (s *routineService) Delete(ctx context.Context, idOrName string, purge bool) error {
	r, err := s.store.Get(idOrName)
	if err != nil {
		return err
	}
	if _, err := s.store.Delete(r.ID); err != nil {
		return err
	}
	if err := s.state.Delete(r.ID); err != nil {
		return err
	}
	if purge {
		if err := os.RemoveAll(filepath.Join(routines.DataDir(), r.ID)); err != nil {
			return err
		}
	}
	return nil
}

func (s *routineService) Logs(ctx context.Context, idOrName string) ([]RoutineLogRef, error) {
	r, err := s.store.Get(idOrName)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(routines.LogDir(r.ID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	refs := make([]RoutineLogRef, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".md" && ext != ".jsonl" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		refs = append(refs, RoutineLogRef{
			Path:     filepath.Join(routines.LogDir(r.ID), entry.Name()),
			Name:     entry.Name(),
			ModTime:  info.ModTime(),
			IsResult: ext == ".md",
		})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].ModTime.After(refs[j].ModTime) })
	return refs, nil
}

func (s *routineService) ReadLog(ctx context.Context, ref RoutineLogRef) (string, error) {
	data, err := os.ReadFile(ref.Path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *routineService) Run(ctx context.Context, req RoutineRunRequest) (RoutineRunResult, error) {
	r, err := s.store.Get(req.IDOrName)
	if err != nil {
		return RoutineRunResult{}, err
	}
	trigger := req.Trigger
	if trigger == "" {
		trigger = routines.TriggerManual
	}

	// A cross-process run lock prevents overlapping runs of the same routine
	// (manual + scheduled, or a run that outlived a daemon restart). If another
	// run holds it, skip cleanly without recording a new run.
	release, err := routines.AcquireRunLock(r.ID)
	if err != nil {
		if errors.Is(err, routines.ErrRunInProgress) {
			return RoutineRunResult{Routine: r}, routines.ErrRunInProgress
		}
		return RoutineRunResult{}, err
	}
	defer release()

	eff := s.resolve(r)
	logDir := routines.LogDir(r.ID)

	meta := buildMeta(string(RunKindRoutine), eff.Provider, eff.Model, eff.WorkingDir, r.Prompt)
	meta.RoutineID = r.ID
	meta.TaskTimeout = formatTaskTimeout(eff.Timeout)
	recorder := sessionlog.New(logDir, meta)
	recorder.Write(sessionlog.Record{Type: sessionlog.RecordRequest, Text: r.Prompt})

	taskReq := TaskRequest{
		Message:      r.Prompt,
		Provider:     eff.Provider,
		Model:        eff.Model,
		WorkingDir:   eff.WorkingDir,
		Deny:         r.Deny,
		AutoApprove:  true,
		Timeout:      eff.Timeout,
		TokenBudget:  eff.TokenBudget,
		MaxTurns:     eff.MaxTurns,
		MaxToolCalls: eff.MaxToolCalls,
		EnabledTools: r.Tools,
		// With no explicit tool list, a routine disables external-facing tools by
		// default; naming tools (r.Tools != nil) is an explicit allow-list instead.
		DisableExternalTools: r.Tools == nil,
		Config:               s.cfg,
	}

	onStep := func(step internalagent.TaskStep) { recorder.Write(taskStepToRecord(step)) }
	onStatus := func(status internalagent.TaskStatusEvent) { recorder.Write(taskStatusToRecord(status)) }
	onProgress := func(progress internalagent.TaskProgressEvent) { recorder.Write(taskProgressToRecord(progress)) }

	start := time.Now().UTC()
	taskResult, runErr := executeTask(ctx, taskReq, internalagent.UnattendedInteraction{}, onStep, onStatus, onProgress, nil)
	duration := time.Since(start)
	outcome := classifyOutcome(runErr)

	if runErr != nil {
		recorder.Write(sessionlog.Record{Type: sessionlog.RecordFailed, Error: runErr.Error()})
	} else {
		recorder.Write(sessionlog.Record{Type: sessionlog.RecordCompleted, Text: taskResult.Response})
	}

	result := RoutineRunResult{
		Routine:    r,
		Output:     taskResult.Response,
		Outcome:    outcome,
		Err:        runErr,
		Duration:   duration,
		TokensUsed: taskResult.TokensUsed,
		SessionLog: recorder.Path(),
	}

	resultPath, summaryErr := s.writeSummary(r, eff, result, trigger, start, recorder.RunID())
	if summaryErr == nil {
		result.ResultPath = resultPath
	}

	record := routines.RunRecord{
		LastRunAt:      start,
		LastStatus:     outcome,
		LastDuration:   duration.Round(time.Millisecond).String(),
		LastTrigger:    trigger,
		LastSessionLog: recorder.Path(),
		LastResultPath: result.ResultPath,
		TokensUsed:     result.TokensUsed,
	}
	if runErr != nil {
		record.LastError = runErr.Error()
	}
	if err := s.state.Record(r.ID, record); err != nil {
		return result, err
	}
	return result, nil
}

func (s *routineService) LaunchNotice(ctx context.Context) (string, error) {
	notices, err := s.state.ClaimPending()
	if err != nil {
		return "", err
	}
	if len(notices) == 0 {
		return "", nil
	}
	failures := make([]string, 0, len(notices))
	for _, notice := range notices {
		if notice.Failure {
			failures = append(failures, notice.ID)
		}
	}
	plural := "s"
	if len(notices) == 1 {
		plural = ""
	}
	msg := fmt.Sprintf("%d routine run%s completed since you were last here", len(notices), plural)
	if len(failures) > 0 {
		sort.Strings(failures)
		msg += fmt.Sprintf("; %d failed: %s — see `agent routine logs %s`",
			len(failures), strings.Join(failures, ", "), failures[0])
	}
	return msg + ".", nil
}

// effectiveSettings are the resolved per-run parameters for a routine.
type effectiveSettings struct {
	Provider     string
	Model        string
	WorkingDir   string
	Timeout      time.Duration
	TokenBudget  int
	MaxTurns     int
	MaxToolCalls int
}

func (s *routineService) resolve(r routines.Routine) effectiveSettings {
	defaults := s.cfg.GetRoutineDefaults()
	// Provider/model fall back to the global config defaults so a routine created
	// with only a prompt still runs; an empty provider would otherwise be rejected
	// by the connector. When only the provider is set, use that provider's
	// configured model.
	provider := firstNonEmpty(r.Provider, defaults.Provider, s.cfg.GetDefaultProvider())
	model := firstNonEmpty(r.Model, defaults.Model)
	if model == "" {
		model = s.cfg.GetModelIdForProvider(provider)
	}
	return effectiveSettings{
		Provider:     provider,
		Model:        model,
		WorkingDir:   firstNonEmpty(r.WorkingDir, defaults.WorkingDir),
		Timeout:      resolveDuration(firstNonEmpty(r.Timeout, defaults.Timeout)),
		TokenBudget:  resolveIntPtr(r.TokenBudget, defaults.TokenBudget),
		MaxTurns:     resolveIntPtr(r.MaxTurns, defaults.MaxTurns),
		MaxToolCalls: resolveIntPtr(r.MaxToolCalls, defaults.MaxToolCalls),
	}
}

func (s *routineService) writeSummary(r routines.Routine, eff effectiveSettings, result RoutineRunResult, trigger string, start time.Time, runID string) (string, error) {
	end := start.Add(result.Duration)
	var b strings.Builder
	title := r.Name
	if title == "" {
		title = r.ID
	}
	fmt.Fprintf(&b, "# Routine: %s\n\n", title)
	fmt.Fprintf(&b, "- ID: %s\n", r.ID)
	fmt.Fprintf(&b, "- Schedule: %s\n", describeSchedule(r.Schedule))
	fmt.Fprintf(&b, "- Trigger: %s\n", trigger)
	fmt.Fprintf(&b, "- Provider/Model: %s / %s\n", orNone(eff.Provider), orNone(eff.Model))
	fmt.Fprintf(&b, "- Started: %s\n", start.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Ended: %s\n", end.Format(time.RFC3339))
	fmt.Fprintf(&b, "- Duration: %s\n", result.Duration.Round(time.Millisecond))
	fmt.Fprintf(&b, "- Status: %s\n", result.Outcome)
	fmt.Fprintf(&b, "- Tokens (est.): %d / %s\n", result.TokensUsed, budgetLabel(eff.TokenBudget))
	if result.Err != nil {
		fmt.Fprintf(&b, "\n## Error\n\n%s\n", result.Err.Error())
	}
	fmt.Fprintf(&b, "\n## Output\n\n%s\n", strings.TrimSpace(result.Output))

	name := fmt.Sprintf("%s_%s_%s.md", start.Format("2006-01-02T15-04-05"), result.Outcome, shortRunID(runID))
	path := filepath.Join(routines.LogDir(r.ID), name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	pruneOldArtifacts(routines.LogDir(r.ID), ".md", maxResultSummaries)
	pruneOldArtifacts(routines.LogDir(r.ID), ".jsonl", maxResultSummaries)
	return path, nil
}

func classifyOutcome(err error) string {
	switch {
	case err == nil:
		return routines.OutcomeSuccess
	case errors.Is(err, internalagent.ErrTaskTimeout):
		return routines.OutcomeTimeout
	case errors.Is(err, internalagent.ErrTokenBudgetExceeded):
		return routines.OutcomeTokenExceeded
	default:
		return routines.OutcomeFailed
	}
}

func pruneOldArtifacts(dir, ext string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type fileEntry struct {
		path string
		mod  time.Time
	}
	var files []fileEntry
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ext {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{path: filepath.Join(dir, entry.Name()), mod: info.ModTime()})
	}
	if len(files) <= keep {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	for _, f := range files[keep:] {
		_ = os.Remove(f.path)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// resolveDuration parses a Go duration string. An empty string or parse error
// yields 0 (unlimited); "0" is therefore an explicit unlimited override.
func resolveDuration(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	d, err := time.ParseDuration(value)
	if err != nil || d < 0 {
		return 0
	}
	return d
}

func resolveIntPtr(primary, fallback *int) int {
	if primary != nil {
		return *primary
	}
	if fallback != nil {
		return *fallback
	}
	return 0
}

func orNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(default)"
	}
	return value
}

// shortRunID returns a short, filename-safe fragment of a run id so result
// summaries from runs in the same second do not collide.
func shortRunID(runID string) string {
	cleaned := strings.ReplaceAll(runID, "-", "")
	if len(cleaned) > 8 {
		cleaned = cleaned[:8]
	}
	if cleaned == "" {
		return "run"
	}
	return cleaned
}

func budgetLabel(budget int) string {
	if budget <= 0 {
		return "unlimited"
	}
	return fmt.Sprintf("%d", budget)
}

// describeSchedule renders a cron expression for display. Phase 1 keeps this
// minimal (manual vs. the raw expression); the daemon phase replaces it with a
// proper cron humanizer.
func describeSchedule(schedule string) string {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" {
		return "Manual"
	}
	return schedule
}
