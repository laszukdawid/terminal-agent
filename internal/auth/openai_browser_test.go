package auth

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"testing"
)

func TestGeneratePKCE(t *testing.T) {
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() error = %v", err)
	}
	if pkce.CodeVerifier == "" {
		t.Fatal("expected non-empty verifier")
	}
	if pkce.CodeChallenge == "" {
		t.Fatal("expected non-empty challenge")
	}
	if pkce.CodeVerifier == pkce.CodeChallenge {
		t.Fatal("verifier and challenge should differ")
	}
	if len(pkce.CodeVerifier) < 43 {
		t.Fatalf("verifier too short: %d", len(pkce.CodeVerifier))
	}
}

func TestGenerateState(t *testing.T) {
	s1, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error = %v", err)
	}
	s2, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error = %v", err)
	}
	if s1 == "" || s2 == "" {
		t.Fatal("expected non-empty state")
	}
	if s1 == s2 {
		t.Fatal("states should be different")
	}
}

func TestBuildAuthorizeURL(t *testing.T) {
	raw := buildAuthorizeURL(
		"http://localhost:1455/auth/callback",
		"challenge",
		"state",
		"terminal-agent",
	)

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	q := u.Query()

	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q, want %q", q.Get("response_type"), "code")
	}
	if q.Get("client_id") != openAIClientID {
		t.Errorf("client_id = %q, want %q", q.Get("client_id"), openAIClientID)
	}
	if q.Get("redirect_uri") != "http://localhost:1455/auth/callback" {
		t.Errorf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	if q.Get("code_challenge") != "challenge" {
		t.Errorf("code_challenge = %q, want %q", q.Get("code_challenge"), "challenge")
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q", q.Get("code_challenge_method"))
	}
	if q.Get("state") != "state" {
		t.Errorf("state = %q, want %q", q.Get("state"), "state")
	}
	if q.Get("id_token_add_organizations") != "true" {
		t.Errorf("id_token_add_organizations = %q", q.Get("id_token_add_organizations"))
	}
	if q.Get("codex_cli_simplified_flow") != "true" {
		t.Errorf("codex_cli_simplified_flow = %q", q.Get("codex_cli_simplified_flow"))
	}
	if q.Get("originator") != "terminal-agent" {
		t.Errorf("originator = %q, want %q", q.Get("originator"), "terminal-agent")
	}
}

func TestParseAuthorizationInput(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCode  string
		wantState string
	}{
		{"empty", "", "", ""},
		{"raw code", "abc123", "abc123", ""},
		{"full redirect URL", "http://localhost:1455/auth/callback?code=abc123&state=mystate", "abc123", "mystate"},
		{"hash separator", "abc123#mystate", "abc123", "mystate"},
		{"query string", "code=abc123&state=mystate", "abc123", "mystate"},
		{"code only in query", "code=abc123", "abc123", ""},
		{"whitespace around", "  code=abc123  ", "abc123", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, state := ParseAuthorizationInput(tc.input)
			if code != tc.wantCode {
				t.Errorf("code = %q, want %q", code, tc.wantCode)
			}
			if state != tc.wantState {
				t.Errorf("state = %q, want %q", state, tc.wantState)
			}
		})
	}
}

func TestDecodeJWTPayload(t *testing.T) {
	headerB64 := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	sigB64 := base64.RawURLEncoding.EncodeToString([]byte(`sig`))

	payload := fmt.Sprintf(`{"%s":{"chatgpt_account_id":"acc-1","chatgpt_plan_type":"pro"}}`, jwtClaimPath)
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payload))
	token := fmt.Sprintf("%s.%s.%s", headerB64, payloadB64, sigB64)

	result, err := decodeJWTPayload(token)
	if err != nil {
		t.Fatalf("decodeJWTPayload() error = %v", err)
	}

	authClaim, ok := result[jwtClaimPath]
	if !ok {
		t.Fatal("expected auth claim")
	}
	authMap, ok := authClaim.(map[string]any)
	if !ok {
		t.Fatal("auth claim is not a map")
	}
	if authMap["chatgpt_account_id"] != "acc-1" {
		t.Errorf("account_id = %v, want acc-1", authMap["chatgpt_account_id"])
	}
}

func TestDecodeJWTPayload_MalformedToken(t *testing.T) {
	_, err := decodeJWTPayload("not.a.jwt")
	if err == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestDecodeJWTPayload_MissingAuthClaim(t *testing.T) {
	headerB64 := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(`{}`))
	sigB64 := base64.RawURLEncoding.EncodeToString([]byte(`sig`))
	token := fmt.Sprintf("%s.%s.%s", headerB64, payloadB64, sigB64)

	result, err := decodeJWTPayload(token)
	if err != nil {
		t.Fatalf("decodeJWTPayload() error = %v", err)
	}
	if _, ok := result[jwtClaimPath]; ok {
		t.Fatal("expected no auth claim")
	}
}

func TestExtractJWTAccountMetadata(t *testing.T) {
	headerB64 := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	sigB64 := base64.RawURLEncoding.EncodeToString([]byte(`sig`))

	payload := fmt.Sprintf(`{"%s":{"chatgpt_account_id":"ws-42"}}`, jwtClaimPath)
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payload))
	token := fmt.Sprintf("%s.%s.%s", headerB64, payloadB64, sigB64)

	accountID, _ := extractJWTAccountMetadata(token)
	if accountID != "ws-42" {
		t.Errorf("accountID = %q, want %q", accountID, "ws-42")
	}
}

func TestBindCallbackServer_FindsAvailablePort(t *testing.T) {
	port, err := bindCallbackServer()
	if err != nil {
		t.Fatalf("bindCallbackServer() error = %v", err)
	}
	if port != openAICallbackPort && port != openAICallbackPortAlt {
		t.Fatalf("expected port %d or %d, got %d", openAICallbackPort, openAICallbackPortAlt, port)
	}
}
