package connector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type PerplexityModelId string

const (
	url                            = "https://api.perplexity.ai/chat/completions"
	llama3_8b    PerplexityModelId = "llama-3-8b-instruct"
	llama3_ss32k PerplexityModelId = "llama-3-sonar-small-32k-online"
)

type AnthropicRequest struct {
	Model    PerplexityModelId `json:"model"`
	Messages []Message         `json:"messages"`
}

// Ask sends a question to the Perplexity API
// and returns the response.
func AskPerplexity(question string, modelID *PerplexityModelId) string {
	if modelID == nil || *modelID == "" {
		model := llama3_ss32k
		modelID = &model
	}

	token := os.Getenv("PERPLEXITY_KEY")

	anthropicRequest := AnthropicRequest{
		Model: *modelID,
		Messages: []Message{
			{
				Role:    "system",
				Content: "Be precise and concise.",
			},
			{
				Role:    "user",
				Content: question,
			},
		},
	}

	payload, err := json.Marshal(anthropicRequest)

	if err != nil {
		fmt.Println(err)
		return ""
	}

	req, _ := http.NewRequest("POST", url, bytes.NewReader(payload))

	req.Header.Add("accept", "application/json")
	req.Header.Add("content-type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))

	res, err := http.DefaultClient.Do(req)

	if err != nil {
		fmt.Println(err)
		return ""
	}

	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)

	s_body := string(body)

	return s_body

}
