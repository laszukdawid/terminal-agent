package routines

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// ErrRunInProgress indicates another run already holds a routine's execution lock.
var ErrRunInProgress = errors.New("routine run already in progress")

func runLockPath(id string) string {
	return filepath.Join(LogDir(id), "run.lock")
}

// AcquireRunLock takes a non-blocking, cross-process exclusive lock for a
// routine's execution. It returns ErrRunInProgress when another run (manual,
// scheduled, or a run that survived a daemon restart) holds it. The returned
// release function frees the lock; the OS also releases it if the holding process
// exits, so a crashed run never leaves the routine permanently locked.
func AcquireRunLock(id string) (release func(), err error) {
	if err := os.MkdirAll(LogDir(id), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(runLockPath(id), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrRunInProgress
		}
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
