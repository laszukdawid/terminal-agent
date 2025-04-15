package connector

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	anthropic "github.com/laszukdawid/anthropic-go-sdk/client"
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
	client  *anthropic.ClientWithResponses

	apiKey string
}

type StreamMessage struct {
	Type    string            `json:"type"`
	Message anthropic.Message `json:"message,omitempty"`
	Delta   ContentBlockDelta `json:"delta,omitempty"`
}

type ContentBlockDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func NewAnthropicConnector(modelID *string, execTools map[string]tools.Tool) *AnthropicConnector {
	logger := *utils.GetLogger()
	logger.Debug("Creating new Anthropic connector", zap.Any("model", modelID))

	apiClient, err := anthropic.NewClientWithResponses(apiUrl)
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

func (ac *AnthropicConnector) queryAnthropicStream(ctx context.Context, params anthropic.MessagesPostParams, body anthropic.CreateMessageParams) (string, error) {
	response, err := ac.client.MessagesPost(ctx, &params, body)
	if err != nil {
		return "", err
	}
	ac.logger.Sugar().Debugw("Anthropic response", "status", response.StatusCode)

	reader := bufio.NewReader(response.Body)
	defer response.Body.Close()

	var acc string

	for {
		// Read line by line
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return acc, nil
			}
			return acc, err
		}

		// Skip empty lines
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		// Remove "data: " prefix
		data := strings.TrimPrefix(line, "data: ")

		// Check for end of stream
		if data == "[DONE]" {
			return acc, nil
		}

		// Parse the message
		var streamMsg StreamMessage
		if err := json.Unmarshal([]byte(data), &streamMsg); err != nil {
			return acc, err
		}

		// Handle different message types
		switch streamMsg.Type {
		case "message_start":
			// Handle start of message
			ac.logger.Sugar().Debugf("Started message: %s\n", streamMsg.Message.Id)
		case "content_block_start":
			// Handle start of content block
			ac.logger.Debug("Started content block\n")
		case "content_block_delta":
			// Handle content update
			fmt.Print(streamMsg.Delta.Text)
			acc += streamMsg.Delta.Text
		case "message_delta":
			// Handle message update
			ac.logger.Sugar().Debugln("Message delta")
		case "message_stop":
			// Handle end of message
			ac.logger.Sugar().Debugln("Message complete")
		}
	}

}

func (ac *AnthropicConnector) Query(ctx context.Context, qParams *QueryParams) (string, error) {
	ac.logger.Sugar().Debugw("Query", "model", ac.modelID)
	var content anthropic.InputMessage_Content
	content.FromInputMessageContent0(*qParams.UserPrompt)

	var system anthropic.CreateMessageParams_System
	err := system.FromCreateMessageParamsSystem0(*qParams.SysPrompt)
	if err != nil {
		return "", err
	}

	var model anthropic.Model
	model.FromModel0(ac.modelID)

	body := anthropic.CreateMessageParams{
		MaxTokens: 100,
		System:    &system,
		Messages: []anthropic.InputMessage{
			{Role: "user", Content: content},
		},
		Model:  model,
		Stream: &qParams.Stream,
	}

	params := anthropic.MessagesPostParams{AnthropicVersion: &anthropicVersion, XApiKey: &ac.apiKey}

	if qParams.Stream {
		return ac.queryAnthropicStream(ctx, params, body)
	}

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

	var msg anthropic.Message
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
