package connector

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	u "github.com/laszukdawid/terminal-agent/internal/utils"
)

type PerplexityModelId string

const (
	PerplexityProvider = "perplexity"

	// Perplexity API endpoint
	url = "https://api.perplexity.ai/chat/completions"

	// Supported models: https://docs.perplexity.ai/docs/model-cards
	llama3_ss128k_chat   PerplexityModelId = "llama-3.1-sonar-small-128k-chat"
	llama3_ss128k_online PerplexityModelId = "llama-3.1-sonar-small-128k-online"
	llama3_sl128k_chat   PerplexityModelId = "llama-3.1-sonar-large-128k-chat"
	llama3_sl128k_online PerplexityModelId = "llama-3.1-sonar-large-128k-online"
)

// Errors dependent on the http status code.
var (
	ErrPerplexityBadRequest = errors.New("perplexity - bad request")
	ErrPerplexityForbidden  = errors.New("perplexity - forbidden")
	ErrPerplexityNotFound   = errors.New("perplexity - not found")
	ErrPerplexityInternal   = errors.New("perplexity - internal server error")
)

type PerplexityRequest struct {
	Model    PerplexityModelId `json:"model"`
	Messages []Message         `json:"messages"`
}

type PerplexityConnector struct {
	modelID PerplexityModelId
	token   string
}

func NewPerplexityConnector(modelID *PerplexityModelId) *PerplexityConnector {
	if modelID == nil || *modelID == "" {
		model := llama3_ss128k_online
		modelID = &model
	}

	connector := &PerplexityConnector{
		modelID: *modelID,
		token:   os.Getenv("PERPLEXITY_KEY"),
	}

	return connector
}

// Ask sends a question to the Perplexity API
// and returns the response.
func (c *PerplexityConnector) Query(userPrompt *string, systemPrompt *string) (string, error) {
	u.Logger.Sugar().Debugw("Query", "model", c.modelID, "provider", PerplexityProvider, "userPrompt", *userPrompt)

	anthropicRequest := PerplexityRequest{
		Model: c.modelID,
		Messages: []Message{
			{Role: "system", Content: *systemPrompt},
			{Role: "user", Content: *userPrompt},
		},
	}

	u.Logger.Sugar().Debugw("Request", "anthropicRequest", anthropicRequest)
	payload, err := json.Marshal(anthropicRequest)

	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	req, _ := http.NewRequest("POST", url, bytes.NewReader(payload))

	req.Header.Add("accept", "application/json")
	req.Header.Add("content-type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.token))

	res, err := http.DefaultClient.Do(req)

	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}

	defer res.Body.Close()

	if res.StatusCode == http.StatusUnauthorized {
		return "", ErrPerplexityForbidden
	}
	if res.StatusCode == http.StatusNotFound {
		return "", ErrPerplexityNotFound
	}

	body, _ := io.ReadAll(res.Body)

	u.Logger.Sugar().Debugw("Response", "body", string(body))
	var response PerplexityResponse
	err = json.Unmarshal(body, &response)

	if err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return response.Choices[0].Message.Content, nil

}

func (c *PerplexityConnector) QueryWithTool(userPrompt *string, systemPrompt *string) (string, error) {
	return "", fmt.Errorf("not implemented")
}
