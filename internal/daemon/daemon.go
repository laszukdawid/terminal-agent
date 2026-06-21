// Package daemon is the background scheduler for routines. A single long-lived
// process holds an in-process cron over all enabled, scheduled routines and, when
// one fires, spawns `agent routine run <id> --scheduled` as an isolated
// subprocess. The OS only supervises the daemon itself (see service.go), not each
// routine. The daemon watches the routines definitions file and reloads on change.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/routines"
	log "github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/robfig/cron/v3"
)

// defaultReconcileInterval is how often the daemon re-checks the definitions file
// (a fallback to the fsnotify watch) and refreshes each routine's next run time.
const defaultReconcileInterval = 60 * time.Second

// ErrNotRunning is returned by Stop when no live daemon is found.
var ErrNotRunning = errors.New("daemon not running")

// Daemon schedules and fires routines. Construct with New; tests may build it
// directly and call loadAndSchedule/fire/reload without the Run loop.
type Daemon struct {
	cfg     config.Config
	store   *routines.Store
	state   *routines.StateStore
	runner  RoutineRunner
	exePath string

	reconcileInterval time.Duration
	now               func() time.Time

	mu        sync.Mutex
	cron      *cron.Cron
	schedules map[string]cron.Schedule // id -> parsed schedule (for next-run refresh)
	running   map[string]bool          // single-flight guard per routine id
	lastMod   time.Time                // definitions file modtime for change detection
}

// New builds a daemon backed by the standard routine stores and the running
// binary as the routine runner.
func New(cfg config.Config) (*Daemon, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}
	return &Daemon{
		cfg:               cfg,
		store:             routines.DefaultStore(),
		state:             routines.DefaultStateStore(),
		runner:            newExecRunner(exe, nil),
		exePath:           exe,
		reconcileInterval: defaultReconcileInterval,
		now:               time.Now,
		schedules:         map[string]cron.Schedule{},
		running:           map[string]bool{},
	}, nil
}

// Run holds the single-instance lock, schedules routines, and blocks until the
// context is cancelled or a termination signal is received.
func (d *Daemon) Run(ctx context.Context) error {
	lock, err := acquireSingleInstanceLock()
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := writePIDFile(os.Getpid()); err != nil {
		return err
	}
	defer removePIDFile()

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	d.reload(true)
	defer d.stopCron()

	watcher := d.startWatcher()
	if watcher != nil {
		defer watcher.Close()
	}
	var events chan fsnotify.Event
	if watcher != nil {
		events = watcher.Events
	}

	ticker := time.NewTicker(d.reconcileInterval)
	defer ticker.Stop()

	log.Infow("Routine daemon started", "pid", os.Getpid(), "exe", d.exePath)
	for {
		select {
		case <-ctx.Done():
			log.Infow("Routine daemon stopping")
			return nil
		case <-ticker.C:
			d.reload(false)
			d.refreshNextRuns()
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if filepath.Base(event.Name) == filepath.Base(routines.DefinitionsPath()) {
				d.reload(false)
			}
		}
	}
}

func (d *Daemon) startWatcher() *fsnotify.Watcher {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Warnw("daemon: fsnotify unavailable, using periodic reload only", "error", err)
		return nil
	}
	dir := filepath.Dir(routines.DefinitionsPath())
	_ = os.MkdirAll(dir, 0o755)
	if err := watcher.Add(dir); err != nil {
		log.Warnw("daemon: watching routines dir failed, using periodic reload only", "dir", dir, "error", err)
		watcher.Close()
		return nil
	}
	return watcher
}

// reload rebuilds the schedule when the definitions file changed (or force is set).
func (d *Daemon) reload(force bool) {
	var mod time.Time
	if info, err := os.Stat(routines.DefinitionsPath()); err == nil {
		mod = info.ModTime()
	}
	d.mu.Lock()
	changed := force || !mod.Equal(d.lastMod)
	d.lastMod = mod
	d.mu.Unlock()
	if changed {
		d.loadAndSchedule()
	}
}

// loadAndSchedule rebuilds the cron from the current definitions and publishes
// each routine's next run time.
func (d *Daemon) loadAndSchedule() {
	routineList, err := d.store.List()
	if err != nil {
		log.Warnw("daemon: loading routines failed", "error", err)
		return
	}
	schedules := selectSchedules(routineList, d.cfg.GetRoutinesEnabled())

	c := cron.New()
	for id, sched := range schedules {
		id := id
		c.Schedule(sched, cron.FuncJob(func() { d.fire(id) }))
	}
	c.Start()

	d.mu.Lock()
	old := d.cron
	d.cron = c
	d.schedules = schedules
	d.mu.Unlock()
	if old != nil {
		old.Stop()
	}

	d.refreshNextRuns()
	log.Infow("daemon: scheduled routines", "count", len(schedules), "enabled", d.cfg.GetRoutinesEnabled())
}

// selectSchedules parses the cron schedule of every enabled routine that has one,
// keyed by id. A disabled global toggle yields an empty set. Invalid cron
// expressions are logged and skipped.
func selectSchedules(routineList []routines.Routine, enabled bool) map[string]cron.Schedule {
	schedules := map[string]cron.Schedule{}
	if !enabled {
		return schedules
	}
	for _, r := range routineList {
		if !r.Enabled || strings.TrimSpace(r.Schedule) == "" {
			continue
		}
		sched, err := cron.ParseStandard(r.Schedule)
		if err != nil {
			log.Warnw("daemon: invalid cron schedule, skipping routine", "routine", r.ID, "schedule", r.Schedule, "error", err)
			continue
		}
		schedules[r.ID] = sched
	}
	return schedules
}

// fire runs one routine, enforcing single-flight: a routine whose previous run is
// still in progress is skipped rather than run concurrently.
func (d *Daemon) fire(id string) {
	d.mu.Lock()
	if d.running[id] {
		d.mu.Unlock()
		log.Infow("daemon: skipping fire, routine still running", "routine", id)
		return
	}
	d.running[id] = true
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		delete(d.running, id)
		d.mu.Unlock()
		d.refreshNextRun(id)
	}()

	log.Infow("daemon: running routine", "routine", id)
	if err := d.runner.Run(context.Background(), id); err != nil {
		// The run records its own failure; this is a subprocess-level diagnostic.
		log.Warnw("daemon: routine run subprocess returned an error", "routine", id, "error", err)
	}
}

func (d *Daemon) refreshNextRuns() {
	d.mu.Lock()
	ids := make([]string, 0, len(d.schedules))
	for id := range d.schedules {
		ids = append(ids, id)
	}
	d.mu.Unlock()
	for _, id := range ids {
		d.refreshNextRun(id)
	}
}

func (d *Daemon) refreshNextRun(id string) {
	d.mu.Lock()
	sched, ok := d.schedules[id]
	d.mu.Unlock()
	if !ok {
		return
	}
	if err := d.state.SetNextRun(id, sched.Next(d.now())); err != nil {
		log.Warnw("daemon: publishing next run time failed", "routine", id, "error", err)
	}
}

func (d *Daemon) stopCron() {
	d.mu.Lock()
	c := d.cron
	d.cron = nil
	d.mu.Unlock()
	if c != nil {
		c.Stop()
	}
}

// Status reports whether a daemon is currently running and its PID.
type Status struct {
	Running bool
	PID     int
}

// CurrentStatus reads the PID file and checks liveness.
func CurrentStatus() (Status, error) {
	pid, err := readPIDFile()
	if err != nil {
		return Status{}, err
	}
	return Status{Running: pid != 0 && processAlive(pid), PID: pid}, nil
}

// Stop signals a running daemon to terminate gracefully.
func Stop() error {
	pid, err := readPIDFile()
	if err != nil {
		return err
	}
	if pid == 0 || !processAlive(pid) {
		return ErrNotRunning
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}
