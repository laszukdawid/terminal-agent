package auth

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

var (
	ErrAuthNotConfigured     = errors.New("OpenAI authentication not configured. Set OPENAI_API_KEY or run 'agent auth login openai --api-key'.")
	ErrOpenAIOAuthConfigured = errors.New("Stored OpenAI OAuth login is configured, but the OpenAI Codex runtime integration is not implemented yet.")
	ErrUnsupportedCredential = errors.New("unsupported stored auth credential")
	ErrUnsupportedProvider   = errors.New("unsupported auth provider")
)

func NormalizeProvider(provider string) string {
	return strings.TrimSpace(strings.ToLower(provider))
}

func ValidateProvider(provider string) error {
	if NormalizeProvider(provider) != ProviderOpenAI {
		return fmt.Errorf("%w: %s", ErrUnsupportedProvider, provider)
	}
	return nil
}

func (m *Manager) SaveAPIKey(provider, key string) error {
	if err := ValidateProvider(provider); err != nil {
		return err
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

	credential, source, configured, err := m.lookupOpenAICredential()
	if err != nil {
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
	credential, source, configured, err := m.lookupOpenAICredential()
	if err != nil {
		return ResolvedAuth{}, err
	}
	if !configured {
		return ResolvedAuth{}, ErrAuthNotConfigured
	}

	switch credential.Type {
	case CredentialTypeAPIKey:
		if strings.TrimSpace(credential.Key) == "" {
			return ResolvedAuth{}, ErrAuthNotConfigured
		}
		return ResolvedAuth{
			Provider: ProviderOpenAI,
			Type:     CredentialTypeAPIKey,
			Source:   source,
			Token:    credential.Key,
		}, nil
	case CredentialTypeOAuth:
		return ResolvedAuth{}, ErrOpenAIOAuthConfigured
	default:
		return ResolvedAuth{}, fmt.Errorf("%w: %s", ErrUnsupportedCredential, credential.Type)
	}
}

func (m *Manager) lookupOpenAICredential() (Credential, string, bool, error) {
	if envKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); envKey != "" {
		return Credential{
			Type: CredentialTypeAPIKey,
			Key:  envKey,
		}, SourceEnvironment, true, nil
	}

	authFile, err := m.Load()
	if err != nil {
		return Credential{}, "", false, err
	}

	credential, exists := authFile[ProviderOpenAI]
	if !exists {
		return Credential{}, "", false, nil
	}

	return credential, SourceStored, true, nil
}
