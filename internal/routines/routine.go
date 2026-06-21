// Package routines holds the persistent model for Terminal Agent routines:
// scheduled, unattended agent runs. It owns the routine definition type, the
// on-disk definitions store (config dir), and the run-status state store (data
// dir). It is a leaf package with no dependency on the app, agent, or config
// packages so the CLI, GUI, app layer, and daemon can all share it.
package routines

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Routine is a saved unit of scheduled work. Empty/nil fields mean "unset": the
// run resolves them against the configured routine defaults and then the
// built-in product defaults. Durations are Go duration strings; pointer fields
// distinguish "unset" from a deliberate zero.
type Routine struct {
	ID           string    `json:"id"`
	Name         string    `json:"name,omitempty"`
	Prompt       string    `json:"prompt"`
	Schedule     string    `json:"schedule,omitempty"` // cron expression; empty = manual-only
	Provider     string    `json:"provider,omitempty"`
	Model        string    `json:"model,omitempty"`
	Timeout      string    `json:"timeout,omitempty"` // Go duration; "0" = unlimited
	TokenBudget  *int      `json:"token_budget,omitempty"`
	MaxTurns     *int      `json:"max_turns,omitempty"`
	MaxToolCalls *int      `json:"max_tool_calls,omitempty"`
	WorkingDir   string    `json:"working_dir,omitempty"`
	Tools        []string  `json:"tools,omitempty"` // enabled tool names; nil = default policy (external off)
	Deny         []string  `json:"deny,omitempty"`  // routine-scoped deny rules, highest priority
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

// idPattern constrains routine IDs to filesystem- and unit-name-safe slugs so
// they can be used verbatim in log directory and OS service file names.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Validate checks the invariants a routine must satisfy to be stored.
func (r Routine) Validate() error {
	if strings.TrimSpace(r.Prompt) == "" {
		return fmt.Errorf("routine prompt cannot be empty")
	}
	if !idPattern.MatchString(r.ID) {
		return fmt.Errorf("routine id %q must match %s", r.ID, idPattern.String())
	}
	return nil
}

// PromptPreview returns the first n characters of the prompt (rune-safe),
// collapsed to a single line, for list displays.
func (r Routine) PromptPreview(n int) string {
	preview := strings.Join(strings.Fields(r.Prompt), " ")
	runes := []rune(preview)
	if len(runes) <= n {
		return preview
	}
	return strings.TrimSpace(string(runes[:n])) + "…"
}

// Slugify converts a human name into a candidate routine ID. It lowercases,
// replaces runs of non-alphanumeric characters with a single hyphen, and trims
// leading/trailing hyphens. An empty or fully-invalid name yields "routine".
func Slugify(name string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" || !idPattern.MatchString(slug) {
		return "routine"
	}
	return slug
}

// UniqueID returns a routine ID derived from name (or base "routine") that does
// not collide with taken(id). Collisions get a numeric suffix.
func UniqueID(name string, taken func(string) bool) string {
	base := Slugify(name)
	if taken == nil || !taken(base) {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !taken(candidate) {
			return candidate
		}
	}
}
