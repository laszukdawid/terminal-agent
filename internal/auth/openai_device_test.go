package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestDeviceUserCode_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != deviceAuthUserCodePath {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["client_id"] != openAIClientID {
			t.Errorf("client_id = %q, want %q", body["client_id"], openAIClientID)
		}

		json.NewEncoder(w).Encode(deviceUserCodeResponse{
			DeviceAuthID: "dev-1",
			UserCode:     "CODE42",
			Interval:     5,
		})
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := requestDeviceUserCode(client, ts.URL)
	if err != nil {
		t.Fatalf("requestDeviceUserCode() error = %v", err)
	}
	if resp.DeviceAuthID != "dev-1" {
		t.Errorf("DeviceAuthID = %q, want %q", resp.DeviceAuthID, "dev-1")
	}
	if resp.UserCode != "CODE42" {
		t.Errorf("UserCode = %q, want %q", resp.UserCode, "CODE42")
	}
	if resp.Interval != 5 {
		t.Errorf("Interval = %d, want 5", resp.Interval)
	}
}

func TestRequestDeviceUserCode_SuccessWithStringInterval(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"device_auth_id":"dev-2","user_code":"CODE99","interval":"7"}`))
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := requestDeviceUserCode(client, ts.URL)
	if err != nil {
		t.Fatalf("requestDeviceUserCode() error = %v", err)
	}
	if resp.Interval != 7 {
		t.Fatalf("Interval = %d, want 7", resp.Interval)
	}
}

func TestRequestDeviceUserCode_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := requestDeviceUserCode(client, ts.URL)
	if err == nil {
		t.Fatal("expected error for unsupported device flow")
	}
}

func TestRequestDeviceUserCode_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := requestDeviceUserCode(client, ts.URL)
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestPollDeviceToken_Success(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		json.NewEncoder(w).Encode(deviceTokenResponse{
			AuthorizationCode: "auth-code-1",
			CodeChallenge:     "challenge-1",
			CodeVerifier:      "verifier-1",
		})
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := pollDeviceToken(client, ts.URL, "dev-1", "CODE42", 0)
	if err != nil {
		t.Fatalf("pollDeviceToken() error = %v", err)
	}
	if resp.AuthorizationCode != "auth-code-1" {
		t.Errorf("AuthorizationCode = %q", resp.AuthorizationCode)
	}
	if resp.CodeVerifier != "verifier-1" {
		t.Errorf("CodeVerifier = %q", resp.CodeVerifier)
	}
}

func TestPollDeviceToken_Timeout(t *testing.T) {
	originalTimeout := deviceAuthTimeout
	deviceAuthTimeout = 1 * time.Millisecond
	defer func() { deviceAuthTimeout = originalTimeout }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := pollDeviceToken(client, ts.URL, "dev-1", "CODE42", 0)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestPollDeviceToken_UnexpectedStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := pollDeviceToken(client, ts.URL, "dev-1", "CODE42", 1)
	if err == nil {
		t.Fatal("expected error for unexpected status")
	}
}
