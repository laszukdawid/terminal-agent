package platform

import (
	"errors"
	"fmt"
	"os"
)

var ErrAlreadyRunning = errors.New("application already running")

const (
	CommandShow = "show"
	CommandHide = "hide"
)

func PrepareSocket(socketPath string) error {
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	return nil
}
