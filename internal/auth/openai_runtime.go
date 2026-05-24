package auth

import "time"

const (
	OpenAICodexBaseURL          = "https://chatgpt.com/backend-api/codex"
	OpenAIResponsesBetaHeader   = "responses=experimental"
	OpenAIOAuthRefreshThreshold = time.Minute
)

var openAITokenEndpoint = openAITokenURL

func OpenAIOriginator() string {
	return openAIOriginator
}
