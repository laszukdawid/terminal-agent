package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func AuthPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		homeDir = os.Getenv("HOME")
	}
	return filepath.Join(homeDir, ".config", "terminal-agent", "auth.json")
}

type Manager struct {
	path string
}

func NewManager() *Manager {
	return &Manager{path: AuthPath()}
}

func NewManagerWithPath(path string) *Manager {
	return &Manager{path: path}
}

func (m *Manager) Path() string {
	return m.path
}

func (m *Manager) Load() (AuthFile, error) {
	return m.loadUnlocked()
}

func (m *Manager) loadUnlocked() (AuthFile, error) {
	content, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return AuthFile{}, nil
		}
		return nil, fmt.Errorf("failed to read auth file: %w", err)
	}

	if len(content) == 0 {
		return AuthFile{}, nil
	}

	authFile := AuthFile{}
	if err := json.Unmarshal(content, &authFile); err != nil {
		return nil, fmt.Errorf("failed to parse auth file: %w", err)
	}

	if authFile == nil {
		return AuthFile{}, nil
	}

	return authFile, nil
}

func (m *Manager) Save(authFile AuthFile) error {
	return m.withLock(func() error {
		return m.saveUnlocked(authFile)
	})
}

func (m *Manager) saveUnlocked(authFile AuthFile) error {
	if authFile == nil {
		authFile = AuthFile{}
	}

	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create auth directory: %w", err)
	}

	content, err := json.MarshalIndent(authFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode auth file: %w", err)
	}
	content = append(content, '\n')

	tmpFile, err := os.CreateTemp(dir, "auth.json.*")
	if err != nil {
		return fmt.Errorf("failed to create auth temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}

	if err := tmpFile.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("failed to set auth temp file permissions: %w", err)
	}

	if _, err := tmpFile.Write(content); err != nil {
		cleanup()
		return fmt.Errorf("failed to write auth temp file: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("failed to sync auth temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		cleanup()
		return fmt.Errorf("failed to close auth temp file: %w", err)
	}

	if err := os.Rename(tmpPath, m.path); err != nil {
		cleanup()
		return fmt.Errorf("failed to replace auth file: %w", err)
	}

	if err := os.Chmod(m.path, 0o600); err != nil {
		return fmt.Errorf("failed to set auth file permissions: %w", err)
	}

	return nil
}

func (m *Manager) SaveProvider(provider string, credential Credential) error {
	return m.withLock(func() error {
		authFile, err := m.loadUnlocked()
		if err != nil {
			return err
		}
		authFile[provider] = credential
		return m.saveUnlocked(authFile)
	})
}

func (m *Manager) DeleteProvider(provider string) (bool, error) {
	deleted := false
	err := m.withLock(func() error {
		authFile, err := m.loadUnlocked()
		if err != nil {
			return err
		}

		if _, exists := authFile[provider]; !exists {
			if provider == ProviderCodex {
				if legacy, legacyExists := authFile[ProviderOpenAI]; legacyExists && legacy.Type == CredentialTypeOAuth {
					delete(authFile, ProviderOpenAI)
					deleted = true
					return m.saveUnlocked(authFile)
				}
			}
			return nil
		}

		delete(authFile, provider)
		deleted = true
		if provider == ProviderCodex {
			if legacy, exists := authFile[ProviderOpenAI]; exists && legacy.Type == CredentialTypeOAuth {
				delete(authFile, ProviderOpenAI)
			}
		}
		return m.saveUnlocked(authFile)
	})
	if err != nil {
		return false, err
	}

	return deleted, nil
}

func (m *Manager) withLock(fn func() error) error {
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create auth directory: %w", err)
	}

	lockPath := m.path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open auth lock file: %w", err)
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire auth lock: %w", err)
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	return fn()
}
