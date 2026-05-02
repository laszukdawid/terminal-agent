package platform

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestIPCServerHandlesShowCommand(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "ipc.sock")
	commands := make(chan string, 1)

	server, err := Listen(socketPath, func(command string) error {
		commands <- command
		return nil
	})
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := Send(ctx, socketPath, CommandShow); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	select {
	case command := <-commands:
		if command != CommandShow {
			t.Fatalf("command = %q, want %q", command, CommandShow)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for IPC command")
	}
}

func TestSendRejectsUnexpectedResponse(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "ipc.sock")
	server, err := Listen(socketPath, func(command string) error {
		if command != CommandShow {
			t.Fatalf("command = %q, want %q", command, CommandShow)
		}
		return context.Canceled
	})
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := Send(ctx, socketPath, CommandShow); err == nil {
		t.Fatal("Send() error = nil, want non-nil")
	}
}
