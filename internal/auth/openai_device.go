package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	deviceAuthUserCodePath = "/api/accounts/deviceauth/usercode"
	deviceAuthTokenPath    = "/api/accounts/deviceauth/token"
	deviceAuthCallbackPath = "/deviceauth/callback"
	deviceVerificationPath = "/codex/device"
)

var deviceAuthTimeout = 15 * time.Minute

type deviceUserCodeResponse struct {
	DeviceAuthID string `json:"device_auth_id"`
	UserCode     string `json:"user_code"`
	Interval     int64  `json:"interval"`
}

func (r *deviceUserCodeResponse) UnmarshalJSON(data []byte) error {
	type rawDeviceUserCodeResponse struct {
		DeviceAuthID string          `json:"device_auth_id"`
		UserCode     string          `json:"user_code"`
		Interval     json.RawMessage `json:"interval"`
	}

	var raw rawDeviceUserCodeResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*r = deviceUserCodeResponse{
		DeviceAuthID: raw.DeviceAuthID,
		UserCode:     raw.UserCode,
	}

	if len(raw.Interval) == 0 || string(raw.Interval) == "null" {
		return nil
	}

	var numericInterval int64
	if err := json.Unmarshal(raw.Interval, &numericInterval); err == nil {
		r.Interval = numericInterval
		return nil
	}

	var stringInterval string
	if err := json.Unmarshal(raw.Interval, &stringInterval); err == nil {
		parsed, parseErr := strconv.ParseInt(strings.TrimSpace(stringInterval), 10, 64)
		if parseErr != nil {
			return fmt.Errorf("invalid device auth interval %q: %w", stringInterval, parseErr)
		}
		r.Interval = parsed
		return nil
	}

	return fmt.Errorf("invalid device auth interval: %s", string(raw.Interval))
}

type deviceTokenRequest struct {
	DeviceAuthID string `json:"device_auth_id"`
	UserCode     string `json:"user_code"`
}

type deviceTokenResponse struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeChallenge     string `json:"code_challenge"`
	CodeVerifier      string `json:"code_verifier"`
}

func (m *Manager) LoginOpenAIDevice(cfg BrowserLoginConfig) (*OAuthResult, error) {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	issuer := strings.TrimRight(openAIIssuer, "/")

	deviceResp, err := requestDeviceUserCode(httpClient, issuer)
	if err != nil {
		return nil, err
	}

	verificationURL := fmt.Sprintf("%s%s", issuer, deviceVerificationPath)

	fmt.Fprintf(os.Stderr, "\nOpen this URL in your browser:\n\n%s\n\n", verificationURL)
	fmt.Fprintf(os.Stderr, "Enter the following one-time code (expires in 15 minutes):\n\n%s\n\n", deviceResp.UserCode)
	fmt.Fprintf(os.Stderr, "Never share this code. It is a common phishing target.\n\n")

	tokenResp, err := pollDeviceToken(httpClient, issuer, deviceResp.DeviceAuthID, deviceResp.UserCode, deviceResp.Interval)
	if err != nil {
		return nil, err
	}

	redirectURI := issuer + deviceAuthCallbackPath

	result, err := exchangeDeviceCodeForTokens(httpClient, issuer, tokenResp.AuthorizationCode, tokenResp.CodeVerifier, tokenResp.CodeChallenge, redirectURI, cfg.Originator)
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

func requestDeviceUserCode(client *http.Client, issuer string) (*deviceUserCodeResponse, error) {
	body, err := json.Marshal(map[string]string{"client_id": openAIClientID})
	if err != nil {
		return nil, fmt.Errorf("failed to encode device auth request: %w", err)
	}

	resp, err := client.Post(issuer+deviceAuthUserCodePath, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("device auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("device code login is not enabled for this server; use browser login instead")
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device auth request returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var deviceResp deviceUserCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&deviceResp); err != nil {
		return nil, fmt.Errorf("failed to decode device auth response: %w", err)
	}

	if deviceResp.DeviceAuthID == "" || deviceResp.UserCode == "" {
		return nil, fmt.Errorf("device auth response missing required fields")
	}

	if deviceResp.Interval <= 0 {
		deviceResp.Interval = 5
	}

	return &deviceResp, nil
}

func pollDeviceToken(client *http.Client, issuer, deviceAuthID, userCode string, interval int64) (*deviceTokenResponse, error) {
	pollBody, err := json.Marshal(deviceTokenRequest{
		DeviceAuthID: deviceAuthID,
		UserCode:     userCode,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode device poll request: %w", err)
	}

	deadline := time.Now().Add(deviceAuthTimeout)
	sleepDuration := time.Duration(interval) * time.Second

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("device auth timed out after 15 minutes")
		}

		time.Sleep(sleepDuration)

		req, err := http.NewRequest(http.MethodPost, issuer+deviceAuthTokenPath, bytes.NewReader(pollBody))
		if err != nil {
			return nil, fmt.Errorf("failed to build device poll request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			var tokenResp deviceTokenResponse
			if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			if tokenResp.AuthorizationCode == "" || tokenResp.CodeVerifier == "" {
				continue
			}

			return &tokenResp, nil
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
			return nil, fmt.Errorf("device auth polling returned unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
		}
	}
}

func exchangeDeviceCodeForTokens(client *http.Client, issuer, authorizationCode, codeVerifier, codeChallenge, redirectURI, originator string) (*oauthTokenExchangeResult, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", openAIClientID)
	form.Set("code", authorizationCode)
	form.Set("code_verifier", codeVerifier)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest(http.MethodPost, issuer+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to build device token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device token exchange returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tokenResp OpenAITokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode device token response: %w", err)
	}

	if tokenResp.AccessToken == "" || tokenResp.RefreshToken == "" || tokenResp.ExpiresIn == 0 {
		return nil, fmt.Errorf("device token response missing required fields")
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
