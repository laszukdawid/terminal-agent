package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/laszukdawid/terminal-agent/internal/routines"
)

const (
	lockFileName = "daemon.lock"
	pidFileName  = "daemon.pid"
)

// ErrAlreadyRunning is returned when another daemon instance already holds the
// single-instance lock.
var ErrAlreadyRunning = errors.New("daemon already running")

func lockPath() string { return filepath.Join(routines.DataDir(), lockFileName) }
func pidPath() string  { return filepath.Join(routines.DataDir(), pidFileName) }

// acquireSingleInstanceLock takes an exclusive, non-blocking advisory lock so only
// one daemon runs at a time. The returned file must stay open for the daemon's
// lifetime; closing it releases the lock.
func acquireSingleInstanceLock() (*os.File, error) {
	if err := os.MkdirAll(routines.DataDir(), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockPath(), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("lock %s: %w", lockPath(), err)
	}
	return f, nil
}

func writePIDFile(pid int) error {
	if err := os.MkdirAll(routines.DataDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(pidPath(), []byte(strconv.Itoa(pid)), 0o644)
}

func removePIDFile() {
	_ = os.Remove(pidPath())
}

// readPIDFile returns the recorded daemon PID, or 0 with no error when no PID
// file exists.
func readPIDFile() (int, error) {
	data, err := os.ReadFile(pidPath())
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid pid file %s: %w", pidPath(), err)
	}
	return pid, nil
}

// processAlive reports whether a process with the given PID exists. Signal 0
// performs no delivery but still validates the target.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	// EPERM means the process exists but is owned by another user.
	return errors.Is(err, syscall.EPERM)
}
