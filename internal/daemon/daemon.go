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

	mu            sync.Mutex
	cron          *cron.Cron
	schedules     map[string]cron.Schedule // id -> parsed schedule (for next-run refresh)
	running       map[string]bool          // single-flight guard per routine id
	lastMod       time.Time                // definitions file modtime for change detection
	lastConfigMod time.Time                // config file modtime (global toggle changes)
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
	var watchErrors chan error
	if watcher != nil {
		events = watcher.Events
		watchErrors = watcher.Errors
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
		case _, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			// reload() decides relevance by comparing the definitions and config
			// file modtimes, so any change in the watched directory is funneled here.
			d.reload(false)
		case err, ok := <-watchErrors:
			// fsnotify requires draining Errors or the watcher can stall; the
			// reconcile ticker still bounds staleness if the watch degrades.
			if !ok {
				watchErrors = nil
				continue
			}
			log.Warnw("daemon: file watch error", "error", err)
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

// reload rebuilds the schedule when the definitions file or the config file
// changed (or force is set). Watching the config file lets a change to the global
// routines toggle take effect without a daemon restart.
func (d *Daemon) reload(force bool) {
	routinesMod := fileModTime(routines.DefinitionsPath())
	configMod := fileModTime(config.ConfigPath())
	d.mu.Lock()
	changed := force || !routinesMod.Equal(d.lastMod) || !configMod.Equal(d.lastConfigMod)
	d.lastMod = routinesMod
	d.lastConfigMod = configMod
	d.mu.Unlock()
	if changed {
		d.loadAndSchedule()
	}
}

func fileModTime(path string) time.Time {
	if info, err := os.Stat(path); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}

// loadAndSchedule reloads configuration, rebuilds the cron from the current
// definitions, and publishes each routine's next run time. A failure to read the
// definitions leaves the previous (last-good) schedule in place so a malformed
// edit degrades gracefully rather than halting all automation.
func (d *Daemon) loadAndSchedule() {
	if cfg, err := config.LoadConfig(); err == nil {
		d.cfg = cfg
	} else {
		log.Warnw("daemon: reloading config failed, keeping previous config", "error", err)
	}

	routineList, err := d.store.List()
	if err != nil {
		log.Errorw("daemon: reading routines failed, keeping previous schedule", "error", err)
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
	oldSchedules := d.schedules
	d.cron = c
	d.schedules = schedules
	d.mu.Unlock()
	if old != nil {
		old.Stop()
	}

	// Clear the published next-run time for routines that are no longer scheduled
	// (disabled or removed) so stale "next run" data does not linger.
	for id := range oldSchedules {
		if _, stillScheduled := schedules[id]; !stillScheduled {
			if err := d.state.SetNextRun(id, time.Time{}); err != nil {
				log.Warnw("daemon: clearing stale next run failed", "routine", id, "error", err)
			}
		}
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

// CurrentStatus reports whether a daemon is running, using the single-instance
// lock as the source of truth so a stale PID file is never reported as running.
func CurrentStatus() (Status, error) {
	running, pid, err := daemonRunning()
	if err != nil {
		return Status{}, err
	}
	if !running {
		removePIDFile() // clean up a stale PID file from a crashed daemon
	}
	return Status{Running: running, PID: pid}, nil
}

// Stop signals the running daemon to terminate gracefully. It relies on the lock
// to confirm a daemon is actually running before signaling any PID.
func Stop() error {
	running, pid, err := daemonRunning()
	if err != nil {
		return err
	}
	if !running {
		removePIDFile()
		return ErrNotRunning
	}
	if pid <= 0 {
		return fmt.Errorf("daemon is running but its PID is unknown")
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}
