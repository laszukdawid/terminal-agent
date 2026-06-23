package daemon

import (
	"context"
	"io"
	"os/exec"
)

// Subcommand pieces used to invoke a scheduled routine run as a subprocess.
const (
	routineSubcommand = "routine"
	runSubcommand     = "run"
	scheduledFlag     = "--scheduled"
)

// RoutineRunner runs a single routine. The default implementation spawns the
// Terminal Agent binary as an isolated subprocess so a crash, hang, or model
// failure in one run cannot affect the daemon or other runs. Tests inject a fake.
type RoutineRunner interface {
	// Run blocks until the routine run completes and returns its error (if any).
	Run(ctx context.Context, routineID string) error
}

// execRunner spawns `<exePath> routine run <id> --scheduled`. The child records
// its own JSONL transcript, result summary, and status entry through the normal
// run path, so its stdout/stderr are only diagnostic and default to discard.
type execRunner struct {
	exePath string
	output  io.Writer
}

func newExecRunner(exePath string, output io.Writer) execRunner {
	if output == nil {
		output = io.Discard
	}
	return execRunner{exePath: exePath, output: output}
}

func (r execRunner) Run(ctx context.Context, routineID string) error {
	// Intentionally not bound to the daemon's context: an in-flight routine run is
	// an independent process that should finish even if the daemon is stopping.
	cmd := exec.Command(r.exePath, routineSubcommand, runSubcommand, routineID, scheduledFlag)
	cmd.Stdout = r.output
	cmd.Stderr = r.output
	return cmd.Run()
}
