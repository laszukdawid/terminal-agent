package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

type Instance struct {
	lockFile   *os.File
	SocketPath string
	LockPath   string
}

func Acquire(appID string) (*Instance, error) {
	baseDir := RuntimeDir(appID)
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, err
	}

	lockPath := filepath.Join(baseDir, "instance.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lockFile.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("acquire instance lock: %w", err)
	}

	return &Instance{
		lockFile:   lockFile,
		SocketPath: filepath.Join(baseDir, "ipc.sock"),
		LockPath:   lockPath,
	}, nil
}

func RuntimeDir(appID string) string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		if runtime.GOOS == "darwin" {
			// On macOS, os.TempDir() returns a process-specific TMPDIR that varies
			// between launch contexts (terminal, Shortcuts.app, Spotlight). Using it
			// would make IPC between those contexts impossible and can exceed the
			// 104-byte Unix socket path limit. Use a fixed, short, user-scoped path.
			runtimeDir = fmt.Sprintf("/tmp/terminal-agent-u%d", os.Getuid())
		} else {
			runtimeDir = os.TempDir()
		}
	}
	return filepath.Join(runtimeDir, appID)
}

func (i *Instance) Close() error {
	var errs []error
	if i.lockFile != nil {
		if err := syscall.Flock(int(i.lockFile.Fd()), syscall.LOCK_UN); err != nil {
			errs = append(errs, err)
		}
		if err := i.lockFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
