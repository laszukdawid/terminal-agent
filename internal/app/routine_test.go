package app

import (
	"context"
	"testing"
	"time"

	internalagent "github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/routines"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoutineResolvePrecedence(t *testing.T) {
	svc := &routineService{cfg: config.NewDefaultConfig()}

	t.Run("falls back to built-in defaults", func(t *testing.T) {
		eff := svc.resolve(routines.Routine{Prompt: "x"})
		assert.Equal(t, 15*time.Minute, eff.Timeout)
		assert.Equal(t, config.DefaultRoutineTokenBudget, eff.TokenBudget)
		assert.Equal(t, 0, eff.MaxTurns, "unset step budget yields 0 so the agent default applies")
		// Provider/model must fall back to the global config defaults so a routine
		// created with only a prompt still resolves a usable connector.
		cfg := config.NewDefaultConfig()
		assert.Equal(t, cfg.GetDefaultProvider(), eff.Provider)
		assert.NotEmpty(t, eff.Provider)
		assert.Equal(t, cfg.GetModelIdForProvider(cfg.GetDefaultProvider()), eff.Model)
		assert.NotEmpty(t, eff.Model)
	})

	t.Run("routine fields win over defaults", func(t *testing.T) {
		budget := 500
		turns := 7
		eff := svc.resolve(routines.Routine{
			Prompt:      "x",
			Timeout:     "5m",
			TokenBudget: &budget,
			MaxTurns:    &turns,
			Provider:    "anthropic",
			Model:       "claude-3-5-haiku-latest",
		})
		assert.Equal(t, 5*time.Minute, eff.Timeout)
		assert.Equal(t, 500, eff.TokenBudget)
		assert.Equal(t, 7, eff.MaxTurns)
		assert.Equal(t, "anthropic", eff.Provider)
		assert.Equal(t, "claude-3-5-haiku-latest", eff.Model)
	})

	t.Run("explicit zero timeout means unlimited", func(t *testing.T) {
		eff := svc.resolve(routines.Routine{Prompt: "x", Timeout: "0"})
		assert.Equal(t, time.Duration(0), eff.Timeout)
	})
}

func TestClassifyOutcome(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "success", err: nil, want: routines.OutcomeSuccess},
		{name: "timeout", err: internalagent.ErrTaskTimeout, want: routines.OutcomeTimeout},
		{name: "token", err: internalagent.ErrTokenBudgetExceeded, want: routines.OutcomeTokenExceeded},
		{name: "other", err: assert.AnError, want: routines.OutcomeFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyOutcome(tt.err))
		})
	}
}

func TestDescribeSchedule(t *testing.T) {
	assert.Equal(t, "Manual", describeSchedule(""))
	assert.Equal(t, "Manual", describeSchedule("   "))
	assert.Equal(t, "0 9 * * 1-5", describeSchedule("0 9 * * 1-5"))
}

func TestRunRoutineRecordsFailureAndWritesArtifacts(t *testing.T) {
	t.Setenv(routines.DataDirEnv, t.TempDir())
	t.Setenv(routines.DefinitionsFileEnv, t.TempDir()+"/routines.json")
	cfg := config.NewDefaultConfig()

	// An unknown provider makes runtime construction fail deterministically, so we
	// exercise RunRoutine's orchestration (recording + summary) without a live LLM.
	store := routines.NewStore(routines.DefinitionsPath())
	require.NoError(t, store.Upsert(routines.Routine{
		ID:       "broken",
		Prompt:   "do something",
		Provider: "nonexistent-provider",
		Enabled:  true,
	}))

	svc := NewRoutineService(cfg)
	result, err := svc.Run(context.Background(), RoutineRunRequest{IDOrName: "broken"})
	require.NoError(t, err, "Run returns nil even when the routine itself failed")
	assert.Equal(t, routines.OutcomeFailed, result.Outcome)
	require.Error(t, result.Err)

	rec, ok, err := routines.NewStateStore(routines.StatePath()).Get("broken")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, routines.OutcomeFailed, rec.LastStatus)
	assert.NotEmpty(t, rec.LastError)

	logs, err := svc.Logs(context.Background(), "broken")
	require.NoError(t, err)
	hasSummary := false
	for _, ref := range logs {
		if ref.IsResult {
			hasSummary = true
		}
	}
	assert.True(t, hasSummary, "a result summary should be written for the failed run")
}

func TestRunRoutineSkipsWhenAlreadyRunning(t *testing.T) {
	t.Setenv(routines.DataDirEnv, t.TempDir())
	t.Setenv(routines.DefinitionsFileEnv, t.TempDir()+"/routines.json")
	cfg := config.NewDefaultConfig()
	store := routines.NewStore(routines.DefinitionsPath())
	require.NoError(t, store.Upsert(routines.Routine{ID: "busy", Prompt: "p", Enabled: true}))

	// Hold the run lock to simulate an in-flight run of the same routine.
	release, err := routines.AcquireRunLock("busy")
	require.NoError(t, err)
	defer release()

	_, runErr := NewRoutineService(cfg).Run(context.Background(), RoutineRunRequest{IDOrName: "busy"})
	assert.ErrorIs(t, runErr, routines.ErrRunInProgress)

	_, hasRun, err := routines.NewStateStore(routines.StatePath()).Get("busy")
	require.NoError(t, err)
	assert.False(t, hasRun, "a skipped run must not record state")
}

func TestLaunchNoticeReportsFailuresOnce(t *testing.T) {
	t.Setenv(routines.DataDirEnv, t.TempDir())
	cfg := config.NewDefaultConfig()
	svc := NewRoutineService(cfg)

	state := routines.NewStateStore(routines.StatePath())
	require.NoError(t, state.Record("nightly", routines.RunRecord{LastRunAt: time.Now().UTC(), LastStatus: routines.OutcomeFailed}))
	require.NoError(t, state.Record("standup", routines.RunRecord{LastRunAt: time.Now().UTC(), LastStatus: routines.OutcomeSuccess}))

	notice, err := svc.LaunchNotice(context.Background())
	require.NoError(t, err)
	assert.Contains(t, notice, "2 routine runs completed")
	assert.Contains(t, notice, "1 failed: nightly")

	// Already reported: a second call is silent.
	notice, err = svc.LaunchNotice(context.Background())
	require.NoError(t, err)
	assert.Empty(t, notice)
}
