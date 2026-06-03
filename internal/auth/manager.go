package auth

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

var (
	ErrAuthNotConfigured      = errors.New("OpenAI authentication not configured. Set OPENAI_API_KEY or run 'agent auth login openai --api-key'.")
	ErrCodexAuthNotConfigured = errors.New("Codex authentication not configured. Run 'agent auth login codex'.")
	ErrOpenAIOAuthConfigured  = errors.New("Stored OpenAI OAuth login is configured. Use the codex provider for OAuth authentication.")
	ErrOpenAIOAuthExpired     = errors.New("Stored Codex OAuth login could not be refreshed. Run 'agent auth login codex' again.")
	ErrUnsupportedCredential  = errors.New("unsupported stored auth credential")
	ErrUnsupportedProvider    = errors.New("unsupported auth provider")
)

func NormalizeProvider(provider string) string {
	return strings.TrimSpace(strings.ToLower(provider))
}

func ValidateProvider(provider string) error {
	switch NormalizeProvider(provider) {
	case ProviderOpenAI, ProviderCodex:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedProvider, provider)
	}
}

func (m *Manager) SaveAPIKey(provider, key string) error {
	if err := ValidateProvider(provider); err != nil {
		return err
	}
	if NormalizeProvider(provider) != ProviderOpenAI {
		return fmt.Errorf("%s does not support API-key auth; use 'agent auth login openai --api-key'", provider)
	}

	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	return m.SaveProvider(NormalizeProvider(provider), Credential{
		Type: CredentialTypeAPIKey,
		Key:  trimmed,
	})
}

func (m *Manager) Status(provider string) (Status, error) {
	if err := ValidateProvider(provider); err != nil {
		return Status{}, err
	}

	status := Status{
		Provider: NormalizeProvider(provider),
		Path:     m.path,
	}

	credential, source, configured, err := m.lookupCredentialForProvider(NormalizeProvider(provider))
	if err != nil {
		if errors.Is(err, ErrOpenAIOAuthConfigured) {
			return status, nil
		}
		return Status{}, err
	}
	if !configured {
		return status, nil
	}

	status.Type = credential.Type
	status.Source = source
	status.Configured = configured
	status.AccountID = credential.AccountID
	status.PlanType = credential.PlanType

	if credential.Type == CredentialTypeOAuth && credential.Expires > 0 {
		status.ExpiresAt = time.UnixMilli(credential.Expires)
		status.Expired = time.Now().After(status.ExpiresAt)
	}

	return status, nil
}

func (m *Manager) ResolveOpenAIAPIKeyAuth() (ResolvedAuth, error) {
	credential, source, configured, err := m.lookupOpenAIAPIKeyCredential()
	if err != nil {
		return ResolvedAuth{}, err
	}
	if !configured || strings.TrimSpace(credential.Key) == "" {
		return ResolvedAuth{}, ErrAuthNotConfigured
	}
	return ResolvedAuth{
		Provider: ProviderOpenAI,
		Type:     CredentialTypeAPIKey,
		Source:   source,
		Token:    credential.Key,
	}, nil
}

func (m *Manager) ResolveOpenAIAuth() (ResolvedAuth, error) {
	return m.ResolveOpenAIAPIKeyAuth()
}

func (m *Manager) ResolveCodexAuth() (ResolvedAuth, error) {
	credential, source, configured, err := m.lookupCodexOAuthCredential()
	if err != nil {
		return ResolvedAuth{}, err
	}
	if !configured {
		return ResolvedAuth{}, ErrCodexAuthNotConfigured
	}

	switch credential.Type {
	case CredentialTypeOAuth:
		refreshedCredential, _, err := m.refreshCodexOAuthIfNeeded()
		if err != nil {
			return ResolvedAuth{}, err
		}
		if strings.TrimSpace(refreshedCredential.Access) == "" {
			return ResolvedAuth{}, ErrCodexAuthNotConfigured
		}
		expiresAt := time.Time{}
		expired := false
		if refreshedCredential.Expires > 0 {
			expiresAt = time.UnixMilli(refreshedCredential.Expires)
			expired = openAIOAuthNeedsRefresh(refreshedCredential, time.Now())
		}
		if strings.TrimSpace(refreshedCredential.AccountID) == "" {
			return ResolvedAuth{}, fmt.Errorf("stored Codex OAuth credential is missing account metadata; run 'agent auth login codex' again")
		}
		return ResolvedAuth{
			Provider:  ProviderCodex,
			Type:      CredentialTypeOAuth,
			Source:    source,
			Token:     refreshedCredential.Access,
			AccountID: refreshedCredential.AccountID,
			PlanType:  refreshedCredential.PlanType,
			ExpiresAt: expiresAt,
			Expired:   expired,
		}, nil
	default:
		return ResolvedAuth{}, fmt.Errorf("%w: %s", ErrUnsupportedCredential, credential.Type)
	}
}

func (m *Manager) lookupCredentialForProvider(provider string) (Credential, string, bool, error) {
	switch provider {
	case ProviderOpenAI:
		return m.lookupOpenAIAPIKeyCredential()
	case ProviderCodex:
		return m.lookupCodexOAuthCredential()
	default:
		return Credential{}, "", false, fmt.Errorf("%w: %s", ErrUnsupportedProvider, provider)
	}
}

func (m *Manager) lookupOpenAIAPIKeyCredential() (Credential, string, bool, error) {
	authFile, err := m.Load()
	if err != nil {
		return Credential{}, "", false, err
	}

	if envKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); envKey != "" {
		return Credential{
			Type: CredentialTypeAPIKey,
			Key:  envKey,
		}, SourceEnvironment, true, nil
	}

	credential, exists := authFile[ProviderOpenAI]
	if exists {
		if credential.Type == CredentialTypeOAuth {
			return Credential{}, "", false, ErrOpenAIOAuthConfigured
		}
		return credential, SourceStored, true, nil
	}

	return Credential{}, "", false, nil
}

func (m *Manager) lookupCodexOAuthCredential() (Credential, string, bool, error) {
	authFile, err := m.Load()
	if err != nil {
		return Credential{}, "", false, err
	}

	if credential, exists := authFile[ProviderCodex]; exists {
		return credential, SourceStored, true, nil
	}

	credential, exists := authFile[ProviderOpenAI]
	if exists && credential.Type == CredentialTypeOAuth {
		return credential, SourceStored, true, nil
	}

	return Credential{}, "", false, nil
}
