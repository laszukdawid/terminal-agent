package daemon

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/routines"
	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fixedNow = time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)

// fakeRunner records routine ids it is asked to run and can block until released.
type fakeRunner struct {
	mu      sync.Mutex
	calls   []string
	started chan string
	release chan struct{}
}

func (r *fakeRunner) Run(_ context.Context, id string) error {
	r.mu.Lock()
	r.calls = append(r.calls, id)
	r.mu.Unlock()
	if r.started != nil {
		r.started <- id
	}
	if r.release != nil {
		<-r.release
	}
	return nil
}

func (r *fakeRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func newTestDaemon(t *testing.T, runner RoutineRunner) *Daemon {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(routines.DataDirEnv, dir)
	t.Setenv(routines.DefinitionsFileEnv, filepath.Join(dir, "routines.json"))
	return &Daemon{
		cfg:               config.NewDefaultConfig(),
		store:             routines.DefaultStore(),
		state:             routines.DefaultStateStore(),
		runner:            runner,
		exePath:           "/fake/agent",
		reconcileInterval: time.Hour,
		now:               func() time.Time { return fixedNow },
		schedules:         map[string]cron.Schedule{},
		running:           map[string]bool{},
	}
}

func TestSelectSchedules(t *testing.T) {
	list := []routines.Routine{
		{ID: "ok", Prompt: "p", Schedule: "0 2 * * *", Enabled: true},
		{ID: "disabled", Prompt: "p", Schedule: "0 2 * * *", Enabled: false},
		{ID: "manual", Prompt: "p", Schedule: "", Enabled: true},
		{ID: "badcron", Prompt: "p", Schedule: "not a cron", Enabled: true},
	}

	t.Run("enabled global: only enabled routines with valid cron", func(t *testing.T) {
		got := selectSchedules(list, true)
		assert.Contains(t, got, "ok")
		assert.NotContains(t, got, "disabled")
		assert.NotContains(t, got, "manual")
		assert.NotContains(t, got, "badcron")
		assert.Len(t, got, 1)
	})

	t.Run("globally disabled: nothing scheduled", func(t *testing.T) {
		assert.Empty(t, selectSchedules(list, false))
	})
}

func TestFireSingleFlight(t *testing.T) {
	runner := &fakeRunner{started: make(chan string), release: make(chan struct{})}
	d := newTestDaemon(t, runner)

	done := make(chan struct{})
	go func() {
		d.fire("a")
		close(done)
	}()

	<-runner.started // first run has begun and holds the single-flight slot
	d.fire("a")      // second fire while the first is in flight must be skipped
	assert.Equal(t, 1, runner.callCount(), "in-flight routine must not run concurrently")

	runner.release <- struct{}{}
	<-done
	assert.Equal(t, 1, runner.callCount())
}

func TestRefreshNextRunPublishesToState(t *testing.T) {
	runner := &fakeRunner{}
	d := newTestDaemon(t, runner)
	sched, err := cron.ParseStandard("0 2 * * *")
	require.NoError(t, err)
	d.schedules["a"] = sched

	d.refreshNextRun("a")

	rec, ok, err := d.state.Get("a")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, sched.Next(fixedNow), rec.NextRunAt)
	assert.Equal(t, fixedNow.Add(2*time.Hour), rec.NextRunAt)
}

func TestLoadAndScheduleRegistersAndPublishes(t *testing.T) {
	runner := &fakeRunner{}
	d := newTestDaemon(t, runner)
	require.NoError(t, d.store.Upsert(routines.Routine{ID: "nightly", Prompt: "p", Schedule: "0 2 * * *", Enabled: true}))
	require.NoError(t, d.store.Upsert(routines.Routine{ID: "off", Prompt: "p", Schedule: "0 2 * * *", Enabled: false}))

	d.loadAndSchedule()
	defer d.stopCron()

	d.mu.Lock()
	_, scheduled := d.schedules["nightly"]
	_, offScheduled := d.schedules["off"]
	d.mu.Unlock()
	assert.True(t, scheduled)
	assert.False(t, offScheduled)

	rec, ok, err := d.state.Get("nightly")
	require.NoError(t, err)
	require.True(t, ok)
	assert.False(t, rec.NextRunAt.IsZero(), "next run should be published")
}

func TestReloadForceLoads(t *testing.T) {
	runner := &fakeRunner{}
	d := newTestDaemon(t, runner)
	require.NoError(t, d.store.Upsert(routines.Routine{ID: "r", Prompt: "p", Schedule: "@hourly", Enabled: true}))

	d.reload(true)
	defer d.stopCron()

	d.mu.Lock()
	_, ok := d.schedules["r"]
	d.mu.Unlock()
	assert.True(t, ok, "force reload should schedule the routine")
}

func TestCurrentStatusTreatsStalePIDAsStopped(t *testing.T) {
	t.Setenv(routines.DataDirEnv, t.TempDir())
	// No daemon is holding the lock, but a stale PID file exists from a crash.
	require.NoError(t, writePIDFile(999999))

	status, err := CurrentStatus()
	require.NoError(t, err)
	assert.False(t, status.Running, "a stale PID file must not be reported as running")

	require.ErrorIs(t, Stop(), ErrNotRunning)
}

func TestLoadAndScheduleReloadsGlobalToggle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".config", "terminal-agent")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"routines":{"enabled":false}}`), 0o644))

	runner := &fakeRunner{}
	d := newTestDaemon(t, runner) // routine stores point at their own temp dir
	require.NoError(t, d.store.Upsert(routines.Routine{ID: "r", Prompt: "p", Schedule: "0 2 * * *", Enabled: true}))

	d.loadAndSchedule() // reloads config from disk (routines globally disabled)
	defer d.stopCron()

	d.mu.Lock()
	count := len(d.schedules)
	d.mu.Unlock()
	assert.Equal(t, 0, count, "global routines.enabled=false must stop scheduling without a restart")
}

func TestRunHoldsSingleInstanceLock(t *testing.T) {
	runner := &fakeRunner{}
	d := newTestDaemon(t, runner)

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- d.Run(ctx) }()

	// Wait until the running daemon has published a live PID.
	require.Eventually(t, func() bool {
		status, err := CurrentStatus()
		return err == nil && status.Running
	}, 2*time.Second, 10*time.Millisecond)

	// A second instance sharing the same data dir cannot acquire the lock.
	d2 := &Daemon{
		cfg: config.NewDefaultConfig(), store: routines.DefaultStore(), state: routines.DefaultStateStore(),
		runner: runner, exePath: "/fake/agent", reconcileInterval: time.Hour,
		now: time.Now, schedules: map[string]cron.Schedule{}, running: map[string]bool{},
	}
	assert.ErrorIs(t, d2.Run(context.Background()), ErrAlreadyRunning)

	cancel()
	require.NoError(t, <-errc)
}
