package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (m *Manager) refreshOpenAIOAuthIfNeeded() (Credential, bool, error) {
	return m.refreshOAuthIfNeeded(ProviderOpenAI)
}

func (m *Manager) refreshCodexOAuthIfNeeded() (Credential, bool, error) {
	return m.refreshOAuthIfNeeded(ProviderCodex)
}

func (m *Manager) refreshOAuthIfNeeded(provider string) (Credential, bool, error) {
	var refreshed bool
	var resolved Credential
	err := m.withLock(func() error {
		authFile, err := m.loadUnlocked()
		if err != nil {
			return err
		}

		credential, exists := authFile[provider]
		legacyCodexCredential := false
		if !exists && provider == ProviderCodex {
			credential, exists = authFile[ProviderOpenAI]
			legacyCodexCredential = exists && credential.Type == CredentialTypeOAuth
		}
		if !exists {
			return ErrAuthNotConfigured
		}
		if credential.Type != CredentialTypeOAuth {
			resolved = credential
			return nil
		}

		if !openAIOAuthNeedsRefresh(credential, time.Now()) {
			if legacyCodexCredential {
				authFile[ProviderCodex] = credential
				delete(authFile, ProviderOpenAI)
				if err := m.saveUnlocked(authFile); err != nil {
					return err
				}
			}
			resolved = credential
			return nil
		}
		if strings.TrimSpace(credential.Refresh) == "" {
			return ErrOpenAIOAuthExpired
		}

		updated, err := refreshOpenAIOAuthCredential(credential)
		if err != nil {
			return err
		}

		authFile[provider] = updated
		if provider == ProviderCodex {
			if legacy, exists := authFile[ProviderOpenAI]; exists && legacy.Type == CredentialTypeOAuth {
				delete(authFile, ProviderOpenAI)
			}
		}
		if err := m.saveUnlocked(authFile); err != nil {
			return err
		}

		resolved = updated
		refreshed = true
		return nil
	})
	if err != nil {
		return Credential{}, false, err
	}
	return resolved, refreshed, nil
}

func openAIOAuthNeedsRefresh(credential Credential, now time.Time) bool {
	if credential.Type != CredentialTypeOAuth {
		return false
	}
	if credential.Expires <= 0 {
		return false
	}
	return !now.Before(time.UnixMilli(credential.Expires).Add(-OpenAIOAuthRefreshThreshold))
}

func refreshOpenAIOAuthCredential(credential Credential) (Credential, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", credential.Refresh)
	form.Set("client_id", openAIClientID)

	req, err := http.NewRequest(http.MethodPost, openAITokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Credential{}, fmt.Errorf("failed to build OAuth refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Credential{}, fmt.Errorf("OAuth refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return Credential{}, fmt.Errorf("OAuth refresh returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tokenResp OpenAITokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return Credential{}, fmt.Errorf("failed to decode OAuth refresh response: %w", err)
	}
	if tokenResp.AccessToken == "" || tokenResp.RefreshToken == "" || tokenResp.ExpiresIn == 0 {
		return Credential{}, fmt.Errorf("OAuth refresh response missing required fields")
	}

	accountID, planType := extractJWTAccountMetadata(tokenResp.AccessToken)
	if strings.TrimSpace(accountID) == "" {
		accountID = credential.AccountID
	}
	if strings.TrimSpace(planType) == "" {
		planType = credential.PlanType
	}

	return Credential{
		Type:      CredentialTypeOAuth,
		Access:    tokenResp.AccessToken,
		Refresh:   tokenResp.RefreshToken,
		Expires:   time.Now().UnixMilli() + tokenResp.ExpiresIn*1000,
		AccountID: accountID,
		PlanType:  planType,
	}, nil
}
