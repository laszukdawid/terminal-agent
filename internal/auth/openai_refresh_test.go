package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRefreshOpenAIOAuthCredentialSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q, want refresh_token", got)
		}
		if got := r.Form.Get("refresh_token"); got != "old-refresh" {
			t.Fatalf("refresh_token = %q, want old-refresh", got)
		}

		_ = json.NewEncoder(w).Encode(OpenAITokenResponse{
			AccessToken:  jwtWithOpenAIAuthClaims("workspace-2", "pro"),
			RefreshToken: "new-refresh",
			ExpiresIn:    3600,
		})
	}))
	defer server.Close()

	originalTokenEndpoint := openAITokenEndpoint
	openAITokenEndpoint = server.URL
	defer func() { openAITokenEndpoint = originalTokenEndpoint }()

	credential, err := refreshOpenAIOAuthCredential(Credential{
		Type:      CredentialTypeOAuth,
		Access:    "old-access",
		Refresh:   "old-refresh",
		Expires:   time.Now().Add(-time.Minute).UnixMilli(),
		AccountID: "workspace-1",
		PlanType:  "plus",
	})
	if err != nil {
		t.Fatalf("refreshOpenAIOAuthCredential() error = %v", err)
	}
	if credential.Refresh != "new-refresh" {
		t.Fatalf("refresh token = %q, want %q", credential.Refresh, "new-refresh")
	}
	if credential.AccountID != "workspace-2" {
		t.Fatalf("account id = %q, want %q", credential.AccountID, "workspace-2")
	}
	if credential.PlanType != "pro" {
		t.Fatalf("plan type = %q, want %q", credential.PlanType, "pro")
	}
	if credential.Expires <= time.Now().UnixMilli() {
		t.Fatal("expected refreshed expiry in the future")
	}
}

func TestResolveCodexAuthRefreshesNearExpiryAndPersists(t *testing.T) {
	manager := newTestManager(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(OpenAITokenResponse{
			AccessToken:  jwtWithOpenAIAuthClaims("workspace-9", "pro"),
			RefreshToken: "rotated-refresh",
			ExpiresIn:    3600,
		})
	}))
	defer server.Close()

	originalTokenEndpoint := openAITokenEndpoint
	openAITokenEndpoint = server.URL
	defer func() { openAITokenEndpoint = originalTokenEndpoint }()

	if err := manager.SaveProvider(ProviderCodex, Credential{
		Type:      CredentialTypeOAuth,
		Access:    "old-access",
		Refresh:   "old-refresh",
		Expires:   time.Now().Add(30 * time.Second).UnixMilli(),
		AccountID: "workspace-1",
		PlanType:  "plus",
	}); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}

	resolved, err := manager.ResolveCodexAuth()
	if err != nil {
		t.Fatalf("ResolveCodexAuth() error = %v", err)
	}
	if resolved.Token == "old-access" {
		t.Fatal("expected refreshed access token")
	}
	if resolved.AccountID != "workspace-9" {
		t.Fatalf("resolved account id = %q, want %q", resolved.AccountID, "workspace-9")
	}

	authFile, err := manager.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	stored := authFile[ProviderCodex]
	if stored.Refresh != "rotated-refresh" {
		t.Fatalf("stored refresh token = %q, want %q", stored.Refresh, "rotated-refresh")
	}
	if stored.AccountID != "workspace-9" {
		t.Fatalf("stored account id = %q, want %q", stored.AccountID, "workspace-9")
	}
}

func TestResolveCodexAuthRefreshFailureLeavesStoredCredentialIntact(t *testing.T) {
	manager := newTestManager(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer server.Close()

	originalTokenEndpoint := openAITokenEndpoint
	openAITokenEndpoint = server.URL
	defer func() { openAITokenEndpoint = originalTokenEndpoint }()

	original := Credential{
		Type:      CredentialTypeOAuth,
		Access:    "old-access",
		Refresh:   "old-refresh",
		Expires:   time.Now().Add(-time.Minute).UnixMilli(),
		AccountID: "workspace-1",
		PlanType:  "plus",
	}
	if err := manager.SaveProvider(ProviderCodex, original); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}

	_, err := manager.ResolveCodexAuth()
	if err == nil {
		t.Fatal("expected refresh failure")
	}

	authFile, loadErr := manager.Load()
	if loadErr != nil {
		t.Fatalf("Load() error = %v", loadErr)
	}
	stored := authFile[ProviderCodex]
	if stored.Access != original.Access || stored.Refresh != original.Refresh || stored.AccountID != original.AccountID {
		t.Fatalf("stored credential changed after failed refresh: %#v", stored)
	}
}

func jwtWithOpenAIAuthClaims(accountID, planType string) string {
	headerB64 := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := fmt.Sprintf(`{"%s":{"chatgpt_account_id":"%s","chatgpt_plan_type":"%s"}}`, jwtClaimPath, accountID, planType)
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payload))
	sigB64 := base64.RawURLEncoding.EncodeToString([]byte(`sig`))
	return fmt.Sprintf("%s.%s.%s", headerB64, payloadB64, sigB64)
}
