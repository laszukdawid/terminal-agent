package auth

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	openAIIssuer          = "https://auth.openai.com"
	openAIAuthorizeURL    = "https://auth.openai.com/oauth/authorize"
	openAITokenURL        = "https://auth.openai.com/oauth/token"
	openAIClientID        = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAICallbackHost    = "127.0.0.1"
	openAICallbackPort    = 1455
	openAICallbackPortAlt = 1457
	openAICallbackPath    = "/auth/callback"
	openAIScope           = "openid profile email offline_access api.connectors.read api.connectors.invoke"
	openAIOriginator      = "terminal-agent"

	jwtClaimPath = "https://api.openai.com/auth"
)

type OpenAITokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type OAuthResult struct {
	Access    string
	Refresh   string
	Expires   int64
	AccountID string
	PlanType  string
}

type BrowserLoginConfig struct {
	OpenBrowser bool
	Originator  string
}

func DefaultBrowserLoginConfig() BrowserLoginConfig {
	return BrowserLoginConfig{
		OpenBrowser: true,
		Originator:  openAIOriginator,
	}
}

func buildAuthorizeURL(redirectURI, codeChallenge, state, originator string) string {
	u, _ := url.Parse(openAIAuthorizeURL)
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", openAIClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", openAIScope)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("originator", originator)
	u.RawQuery = q.Encode()
	return u.String()
}

func (m *Manager) LoginOpenAIBrowser(cfg BrowserLoginConfig) (*OAuthResult, error) {
	pkce, err := GeneratePKCE()
	if err != nil {
		return nil, fmt.Errorf("failed to generate PKCE: %w", err)
	}

	state, err := GenerateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	callbackPort, err := bindCallbackServer()
	if err != nil {
		return nil, fmt.Errorf("failed to bind callback server: %w", err)
	}

	redirectURI := fmt.Sprintf("http://localhost:%d%s", callbackPort, openAICallbackPath)
	authURL := buildAuthorizeURL(redirectURI, pkce.CodeChallenge, state, cfg.Originator)

	fmt.Fprintf(os.Stderr, "\nOpen this URL in your browser to authenticate:\n\n%s\n\n", authURL)
	fmt.Fprintln(os.Stderr, "If the callback does not return automatically, paste the authorization code or full redirect URL here and press Enter.")
	if cfg.OpenBrowser {
		openBrowser(authURL)
	}

	code, err := waitForAuthCode(callbackPort, state)
	if err != nil {
		return nil, err
	}

	result, err := exchangeCodeForTokens(code, pkce.CodeVerifier, redirectURI, cfg.Originator)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().UnixMilli() + result.ExpiresIn*1000

	if err := m.SaveProvider(ProviderOpenAI, Credential{
		Type:      CredentialTypeOAuth,
		Access:    result.AccessToken,
		Refresh:   result.RefreshToken,
		Expires:   expiresAt,
		AccountID: result.AccountID,
		PlanType:  result.PlanType,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist OAuth credential: %w", err)
	}

	return &OAuthResult{
		Access:    result.AccessToken,
		Refresh:   result.RefreshToken,
		Expires:   expiresAt,
		AccountID: result.AccountID,
		PlanType:  result.PlanType,
	}, nil
}

func bindCallbackServer() (int, error) {
	for _, port := range []int{openAICallbackPort, openAICallbackPortAlt} {
		addr := fmt.Sprintf("%s:%d", openAICallbackHost, port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			listener.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("callback ports %d and %d are both unavailable", openAICallbackPort, openAICallbackPortAlt)
}

func waitForAuthCode(port int, expectedState string) (string, error) {
	mux := http.NewServeMux()

	type callbackResult struct {
		code string
		err  error
	}
	resultCh := make(chan callbackResult, 1)
	manualResultCh := make(chan callbackResult, 1)

	mux.HandleFunc(openAICallbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		if q.Get("state") != expectedState {
			http.Error(w, "State mismatch", http.StatusBadRequest)
			resultCh <- callbackResult{err: fmt.Errorf("OAuth callback state mismatch")}
			return
		}

		code := q.Get("code")
		if code == "" {
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			resultCh <- callbackResult{err: fmt.Errorf("OAuth callback missing authorization code")}
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<html><body><h1>Terminal Agent</h1><p>Authentication completed. You can close this window.</p></body></html>`)

		resultCh <- callbackResult{code: code}
	})

	addr := fmt.Sprintf("%s:%d", openAICallbackHost, port)
	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		_ = srv.ListenAndServe()
	}()

	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			input := strings.TrimSpace(scanner.Text())
			if input == "" {
				continue
			}

			code, state := ParseAuthorizationInput(input)
			if state != "" && state != expectedState {
				manualResultCh <- callbackResult{err: fmt.Errorf("OAuth manual input state mismatch")}
				return
			}
			if code == "" {
				manualResultCh <- callbackResult{err: fmt.Errorf("OAuth manual input missing authorization code")}
				return
			}

			manualResultCh <- callbackResult{code: code}
			return
		}
		if err := scanner.Err(); err != nil {
			manualResultCh <- callbackResult{err: fmt.Errorf("failed to read manual OAuth input: %w", err)}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	select {
	case result := <-resultCh:
		return result.code, result.err
	case result := <-manualResultCh:
		return result.code, result.err
	case <-ctx.Done():
		return "", fmt.Errorf("OAuth login timed out; no callback received within 5 minutes")
	}
}

type oauthTokenExchangeResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	AccountID    string
	PlanType     string
}

func exchangeCodeForTokens(code, codeVerifier, redirectURI, originator string) (*oauthTokenExchangeResult, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", openAIClientID)
	form.Set("code", code)
	form.Set("code_verifier", codeVerifier)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest(http.MethodPost, openAITokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tokenResp OpenAITokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" || tokenResp.RefreshToken == "" || tokenResp.ExpiresIn == 0 {
		return nil, fmt.Errorf("token response missing required fields (access_token, refresh_token, expires_in)")
	}

	accountID, planType := extractJWTAccountMetadata(tokenResp.AccessToken)

	return &oauthTokenExchangeResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
		AccountID:    accountID,
		PlanType:     planType,
	}, nil
}

func extractJWTAccountMetadata(accessToken string) (accountID, planType string) {
	payload, err := decodeJWTPayload(accessToken)
	if err != nil {
		return "", ""
	}

	authClaim, ok := payload[jwtClaimPath]
	if !ok {
		return "", ""
	}

	authMap, ok := authClaim.(map[string]any)
	if !ok {
		return "", ""
	}

	if aid, ok := authMap["chatgpt_account_id"].(string); ok {
		accountID = aid
	}

	if pt, ok := authMap["chatgpt_plan_type"].(string); ok {
		planType = pt
	}

	return accountID, planType
}

func decodeJWTPayload(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse JWT payload: %w", err)
	}

	return payload, nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}

func ParseAuthorizationInput(input string) (code, state string) {
	val := strings.TrimSpace(input)
	if val == "" {
		return "", ""
	}

	if u, err := url.Parse(val); err == nil && u.Scheme != "" {
		return u.Query().Get("code"), u.Query().Get("state")
	}

	if idx := strings.IndexByte(val, '#'); idx != -1 {
		code := val[:idx]
		state := val[idx+1:]
		return code, state
	}

	if strings.Contains(val, "code=") {
		q, _ := url.ParseQuery(val)
		return q.Get("code"), q.Get("state")
	}

	return val, ""
}
