package routines

import (
	"fmt"
	"strings"

	"github.com/robfig/cron/v3"
)

// ValidateSchedule reports whether a routine schedule is a usable cron expression.
// An empty schedule is valid and means "manual only". A non-empty schedule must
// parse as a standard 5-field cron expression or a supported @descriptor
// (@hourly, @daily, @weekly, @monthly, @yearly, @every <dur>).
func ValidateSchedule(expr string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	if _, err := cron.ParseStandard(expr); err != nil {
		return fmt.Errorf("invalid cron schedule %q: %w", expr, err)
	}
	return nil
}
