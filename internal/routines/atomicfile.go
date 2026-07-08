package routines

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// withLock runs fn while holding an exclusive advisory lock on lockPath, so
// independently launched processes (scheduled runs and interactive commands)
// serialize their read-modify-write cycles on shared state.
func withLock(lockPath string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock %s: %w", lockPath, err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	return fn()
}
