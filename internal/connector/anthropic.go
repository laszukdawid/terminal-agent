package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/laszukdawid/anthropic-go-sdk/client"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
)

const (
	AnthropicProvider = "anthropic"
	apiUrl            = "https://api.anthropic.com"
)

var anthropicVersion string = "2023-06-01"

// type AnthropicModelID string

type AnthropicConnector struct {
	modelID string
	logger  zap.Logger
	client  *client.ClientWithResponses

	apiKey    string
	execTools map[string]tools.Tool
}

func NewAnthropicConnector(modelID *string, execTools map[string]tools.Tool) *AnthropicConnector {
	logger := *utils.GetLogger()
	logger.Debug("Creating new Anthropic connector", zap.Any("model", modelID))

	apiClient, err := client.NewClientWithResponses(apiUrl)
	if err != nil {
		logger.Fatal("Failed to create client", zap.Error(err))
		return nil
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		logger.Fatal("ANTHROPIC_API_KEY is not set")
		return nil
	}

	return &AnthropicConnector{
		modelID: *modelID,
		logger:  logger, client: apiClient,
		apiKey: apiKey,
	}
}

func (ac *AnthropicConnector) Query(ctx context.Context, qParams *QueryParams) (string, error) {
	ac.logger.Sugar().Debugw("Query", "model", ac.modelID)
	var content client.InputMessage_Content
	content.FromInputMessageContent0(*qParams.UserPrompt)

	var system client.CreateMessageParams_System
	err := system.FromCreateMessageParamsSystem0(*qParams.SysPrompt)
	if err != nil {
		return "", err
	}

	var model client.Model
	model.FromModel0(ac.modelID)

	body := client.CreateMessageParams{
		MaxTokens: 100,
		System:    &system,
		Messages: []client.InputMessage{
			{Role: "user", Content: content},
		},
		Model: model,
	}

	params := client.MessagesPostParams{AnthropicVersion: &anthropicVersion, XApiKey: &ac.apiKey}
	response, err := ac.client.MessagesPost(context.Background(), &params, body)
	if err != nil {
		return "", err
	}

	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("model %s returned response is nil", ac.modelID)
	}
	if response.StatusCode != 200 {
		return "", fmt.Errorf("model %s returned status code %d: %s", ac.modelID, response.StatusCode, string(bodyBytes))
	}

	var msg client.Message
	json.Unmarshal(bodyBytes, &msg)

	var outText string
	for _, block := range msg.Content {
		_txt, _ := block.AsResponseTextBlock()
		outText += _txt.Text
	}

	return outText, nil
}

func (ac *AnthropicConnector) QueryWithTool(ctx context.Context, qParams *QueryParams) (string, error) {
	ac.logger.Sugar().Fatalf("QueryWithTool is not implemented for Anthropic")
	return "", nil
}
