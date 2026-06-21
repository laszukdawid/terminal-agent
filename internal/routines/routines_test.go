package routines

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreUpsertGetDelete(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "routines.json"))

	require.NoError(t, store.Upsert(Routine{ID: "daily", Name: "Daily", Prompt: "do it", Enabled: true}))
	require.NoError(t, store.Upsert(Routine{ID: "weekly", Name: "Weekly", Prompt: "do it weekly", Enabled: false}))

	list, err := store.List()
	require.NoError(t, err)
	assert.Len(t, list, 2)

	got, err := store.Get("daily")
	require.NoError(t, err)
	assert.Equal(t, "Daily", got.Name)
	assert.False(t, got.CreatedAt.IsZero(), "CreatedAt should be stamped")

	// Update preserves CreatedAt and bumps UpdatedAt.
	created := got.CreatedAt
	got.Prompt = "updated"
	require.NoError(t, store.Upsert(got))
	reloaded, err := store.Get("daily")
	require.NoError(t, err)
	assert.Equal(t, "updated", reloaded.Prompt)
	assert.Equal(t, created, reloaded.CreatedAt)

	removed, err := store.Delete("daily")
	require.NoError(t, err)
	assert.True(t, removed)
	_, err = store.Get("daily")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestStoreAddRejectsDuplicateID(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "routines.json"))
	require.NoError(t, store.Add(Routine{ID: "daily", Prompt: "p", Enabled: true}))
	err := store.Add(Routine{ID: "daily", Prompt: "other", Enabled: true})
	assert.ErrorIs(t, err, ErrExists, "Add must not silently overwrite an existing routine")

	got, err := store.Get("daily")
	require.NoError(t, err)
	assert.Equal(t, "p", got.Prompt, "original routine is preserved")
}

func TestStateClaimPendingIsAtomicPerSnapshot(t *testing.T) {
	state := NewStateStore(filepath.Join(t.TempDir(), "state.json"))
	require.NoError(t, state.Record("a", RunRecord{LastRunAt: time.Now().UTC(), LastStatus: OutcomeSuccess}))
	require.NoError(t, state.Record("b", RunRecord{LastRunAt: time.Now().UTC(), LastStatus: OutcomeFailed}))

	claimed, err := state.ClaimPending()
	require.NoError(t, err)
	assert.Len(t, claimed, 2)

	// A run completing after the claim is still reported on the next call (the seen
	// marker only advanced to the newest run in the previous snapshot).
	require.NoError(t, state.Record("c", RunRecord{LastRunAt: time.Now().UTC().Add(time.Second), LastStatus: OutcomeSuccess}))
	claimed, err = state.ClaimPending()
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, "c", claimed[0].ID)

	claimed, err = state.ClaimPending()
	require.NoError(t, err)
	assert.Empty(t, claimed)
}

func TestStoreResolveByNameAndPrefix(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "routines.json"))
	require.NoError(t, store.Upsert(Routine{ID: "report-daily", Name: "Daily Report", Prompt: "x", Enabled: true}))
	require.NoError(t, store.Upsert(Routine{ID: "backup-nightly", Name: "Nightly Backup", Prompt: "y", Enabled: true}))

	tests := []struct {
		name    string
		query   string
		wantID  string
		wantErr bool
	}{
		{name: "exact id", query: "report-daily", wantID: "report-daily"},
		{name: "exact name", query: "Nightly Backup", wantID: "backup-nightly"},
		{name: "unique prefix", query: "backup", wantID: "backup-nightly"},
		{name: "unknown", query: "nope", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.Get(tt.query)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.ID)
		})
	}
}

func TestStoreUpsertRejectsInvalid(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "routines.json"))
	assert.Error(t, store.Upsert(Routine{ID: "ok", Prompt: ""}), "empty prompt should be rejected")
	assert.Error(t, store.Upsert(Routine{ID: "Bad ID", Prompt: "x"}), "invalid id should be rejected")
}

func TestSlugifyAndUniqueID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "spaces", in: "Daily Standup", want: "daily-standup"},
		{name: "symbols", in: "Report: 2026!!", want: "report-2026"},
		{name: "empty", in: "   ", want: "routine"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Slugify(tt.in))
		})
	}

	taken := map[string]bool{"daily-standup": true, "daily-standup-2": true}
	got := UniqueID("Daily Standup", func(id string) bool { return taken[id] })
	assert.Equal(t, "daily-standup-3", got)
}

func TestStateRecordTracksConsecutiveFailures(t *testing.T) {
	state := NewStateStore(filepath.Join(t.TempDir(), "state.json"))

	require.NoError(t, state.Record("r", RunRecord{LastRunAt: time.Now().UTC(), LastStatus: OutcomeFailed}))
	require.NoError(t, state.Record("r", RunRecord{LastRunAt: time.Now().UTC(), LastStatus: OutcomeTimeout}))
	rec, ok, err := state.Get("r")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, 2, rec.ConsecutiveFailures)

	require.NoError(t, state.Record("r", RunRecord{LastRunAt: time.Now().UTC(), LastStatus: OutcomeSuccess}))
	rec, _, _ = state.Get("r")
	assert.Equal(t, 0, rec.ConsecutiveFailures, "success resets the failure streak")
}

func TestStateSetNextRunPreservesHistory(t *testing.T) {
	state := NewStateStore(filepath.Join(t.TempDir(), "state.json"))
	require.NoError(t, state.Record("r", RunRecord{LastStatus: OutcomeSuccess, LastDuration: "1s"}))
	next := time.Now().UTC().Add(time.Hour)
	require.NoError(t, state.SetNextRun("r", next))

	rec, _, err := state.Get("r")
	require.NoError(t, err)
	assert.Equal(t, "1s", rec.LastDuration, "history is preserved")
	assert.WithinDuration(t, next, rec.NextRunAt, time.Second)

	// Recording a new run preserves the daemon-set NextRunAt when not provided.
	require.NoError(t, state.Record("r", RunRecord{LastStatus: OutcomeSuccess}))
	rec, _, _ = state.Get("r")
	assert.WithinDuration(t, next, rec.NextRunAt, time.Second)
}

func TestStatePendingSinceAndMarkSeen(t *testing.T) {
	state := NewStateStore(filepath.Join(t.TempDir(), "state.json"))
	require.NoError(t, state.Record("ok", RunRecord{LastRunAt: time.Now().UTC(), LastStatus: OutcomeSuccess}))
	require.NoError(t, state.Record("bad", RunRecord{LastRunAt: time.Now().UTC(), LastStatus: OutcomeFailed}))

	notices, err := state.PendingSince()
	require.NoError(t, err)
	assert.Len(t, notices, 2)
	failures := 0
	for _, n := range notices {
		if n.Failure {
			failures++
		}
	}
	assert.Equal(t, 1, failures)

	require.NoError(t, state.MarkSeen())
	notices, err = state.PendingSince()
	require.NoError(t, err)
	assert.Empty(t, notices, "marking seen clears pending notices")
}

func TestDisplayStatus(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		rec     RunRecord
		hasRun  bool
		want    string
	}{
		{name: "disabled", enabled: false, want: StatusInactive},
		{name: "enabled no runs", enabled: true, hasRun: false, want: StatusActive},
		{name: "enabled last ok", enabled: true, hasRun: true, rec: RunRecord{LastStatus: OutcomeSuccess}, want: StatusActive},
		{name: "enabled last failed", enabled: true, hasRun: true, rec: RunRecord{LastStatus: OutcomeFailed}, want: StatusError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DisplayStatus(tt.enabled, tt.rec, tt.hasRun))
		})
	}
}

func TestStoreAtomicWriteRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routines.json")
	store := NewStore(path)
	budget := 500
	require.NoError(t, store.Upsert(Routine{ID: "r", Prompt: "p", Tools: []string{"read"}, TokenBudget: &budget, Enabled: true}))

	// A fresh store reading the same file observes the persisted values.
	got, err := NewStore(path).Get("r")
	require.NoError(t, err)
	require.NotNil(t, got.TokenBudget)
	assert.Equal(t, 500, *got.TokenBudget)
	assert.Equal(t, []string{"read"}, got.Tools)
}

func TestPromptPreview(t *testing.T) {
	r := Routine{Prompt: "summarize   the\nlatest commits and report"}
	assert.Equal(t, "summarize the latest…", r.PromptPreview(21))
	assert.Equal(t, "short", Routine{Prompt: "short"}.PromptPreview(20))
}
