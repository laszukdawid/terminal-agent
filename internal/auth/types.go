package auth

import "time"

const (
	CredentialTypeAPIKey = "api_key"
	CredentialTypeOAuth  = "oauth"

	ProviderOpenAI = "openai"
	ProviderCodex  = "codex"

	SourceEnvironment = "environment"
	SourceStored      = "stored"
)

type Credential struct {
	Type      string `json:"type"`
	Key       string `json:"key,omitempty"`
	Access    string `json:"access,omitempty"`
	Refresh   string `json:"refresh,omitempty"`
	Expires   int64  `json:"expires,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	PlanType  string `json:"plan_type,omitempty"`
}

type AuthFile map[string]Credential

type Status struct {
	Provider   string
	Type       string
	Source     string
	Configured bool
	Expired    bool
	ExpiresAt  time.Time
	AccountID  string
	PlanType   string
	Path       string
}

type ResolvedAuth struct {
	Provider  string
	Type      string
	Source    string
	Token     string
	AccountID string
	PlanType  string
	ExpiresAt time.Time
	Expired   bool
}
