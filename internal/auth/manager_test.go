package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	t.Setenv("OPENAI_API_KEY", "")
	return NewManagerWithPath(filepath.Join(t.TempDir(), "auth.json"))
}

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	mgr := newTestManager(t)

	authFile, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(authFile) != 0 {
		t.Fatalf("expected empty auth file, got %#v", authFile)
	}
}

func TestSaveAPIKeyAndStatus(t *testing.T) {
	mgr := newTestManager(t)

	if err := mgr.SaveAPIKey(ProviderOpenAI, "sk-test"); err != nil {
		t.Fatalf("SaveAPIKey() error = %v", err)
	}

	status, err := mgr.Status(ProviderOpenAI)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Configured {
		t.Fatal("expected configured status")
	}
	if status.Type != CredentialTypeAPIKey {
		t.Fatalf("status.Type = %q, want %q", status.Type, CredentialTypeAPIKey)
	}
	if status.Source != SourceStored {
		t.Fatalf("status.Source = %q, want %q", status.Source, SourceStored)
	}

	info, err := os.Stat(mgr.Path())
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o, want 600", info.Mode().Perm())
	}
}

func TestResolveOpenAIAPIKeyAuthPrefersEnvironment(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.SaveAPIKey(ProviderOpenAI, "sk-stored"); err != nil {
		t.Fatalf("SaveAPIKey() error = %v", err)
	}

	t.Setenv("OPENAI_API_KEY", "sk-env")

	resolved, err := mgr.ResolveOpenAIAPIKeyAuth()
	if err != nil {
		t.Fatalf("ResolveOpenAIAPIKeyAuth() error = %v", err)
	}
	if resolved.Source != SourceEnvironment {
		t.Fatalf("resolved.Source = %q, want %q", resolved.Source, SourceEnvironment)
	}
	if resolved.Token != "sk-env" {
		t.Fatalf("resolved.Token = %q, want %q", resolved.Token, "sk-env")
	}
}

func TestResolveOpenAIAPIKeyAuthReturnsStoredAPIKey(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.SaveAPIKey(ProviderOpenAI, "sk-stored"); err != nil {
		t.Fatalf("SaveAPIKey() error = %v", err)
	}

	resolved, err := mgr.ResolveOpenAIAPIKeyAuth()
	if err != nil {
		t.Fatalf("ResolveOpenAIAPIKeyAuth() error = %v", err)
	}
	if resolved.Source != SourceStored {
		t.Fatalf("resolved.Source = %q, want %q", resolved.Source, SourceStored)
	}
	if resolved.Token != "sk-stored" {
		t.Fatalf("resolved.Token = %q, want %q", resolved.Token, "sk-stored")
	}
}

func TestResolveOpenAIAPIKeyAuthReturnsOAuthNotSupported(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.SaveProvider(ProviderOpenAI, Credential{
		Type:    CredentialTypeOAuth,
		Access:  "access-token",
		Refresh: "refresh-token",
		Expires: time.Now().Add(time.Hour).UnixMilli(),
	}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}

	_, err := mgr.ResolveOpenAIAPIKeyAuth()
	if !errors.Is(err, ErrOpenAIOAuthConfigured) {
		t.Fatalf("ResolveOpenAIAPIKeyAuth() error = %v, want %v", err, ErrOpenAIOAuthConfigured)
	}
}

func TestDeleteProvider(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.SaveAPIKey(ProviderOpenAI, "sk-stored"); err != nil {
		t.Fatalf("SaveAPIKey() error = %v", err)
	}

	deleted, err := mgr.DeleteProvider(ProviderOpenAI)
	if err != nil {
		t.Fatalf("DeleteProvider() error = %v", err)
	}
	if !deleted {
		t.Fatal("expected provider to be deleted")
	}

	status, err := mgr.Status(ProviderOpenAI)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Configured {
		t.Fatalf("expected provider to be unconfigured after delete, got %#v", status)
	}
}

func TestResolveOpenAIAPIKeyAuthReturnsNotConfigured(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.ResolveOpenAIAPIKeyAuth()
	if !errors.Is(err, ErrAuthNotConfigured) {
		t.Fatalf("ResolveOpenAIAPIKeyAuth() error = %v, want %v", err, ErrAuthNotConfigured)
	}
}
