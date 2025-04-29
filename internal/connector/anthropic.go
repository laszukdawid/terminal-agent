package connector

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/laszukdawid/terminal-agent/internal/tools"

	"github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
)

const (
	AnthropicProvider = "anthropic"
)

type AnthropicConnector struct {
	modelID string
	logger  zap.Logger
	client  anthropic.Client
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

func NewAnthropicConnector(modelID *string) *AnthropicConnector {
	logger := *utils.GetLogger()
	logger.Debug("Creating new Anthropic connector", zap.Any("model", modelID))

	apiClient := anthropic.NewClient()

	return &AnthropicConnector{
		modelID: *modelID,
		logger:  logger, client: apiClient,
	}
}

func (ac *AnthropicConnector) queryAnthropicStream(ctx context.Context, msgParams anthropic.MessageNewParams) (string, error) {
	// Stream the response
	stream := ac.client.Messages.NewStreaming(ctx, msgParams)
	message := anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		err := message.Accumulate(event)
		if err != nil {
			panic(err)
		}

		switch eventVariant := event.AsAny().(type) {
		case anthropic.ContentBlockDeltaEvent:
			switch deltaVariant := eventVariant.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				fmt.Print(deltaVariant.Text)
			}

		}

		if stream.Err() != nil {
			panic(stream.Err())
		}
	}
	return message.Content[0].AsResponseTextBlock().Text, nil
}

// convertToolsToAnthropic converts internal tool representations to Anthropic's tool format
func convertToolsToAnthropic(tools map[string]tools.Tool) []anthropic.ToolParam {
	var anthropicTools []anthropic.ToolParam
	for _, tool := range tools {
		inputSchema := tool.InputSchema()
		properties, ok := inputSchema["properties"].(map[string]any)
		if !ok {
			// Handle error
			utils.Logger.Error("Failed to convert input schema properties", zap.Any("inputSchema", inputSchema))
			continue
		}
		extraProps := make(map[string]any)
		if required, ok := inputSchema["required"].([]string); ok {
			extraProps["required"] = required
		}
		anthropicTool := anthropic.ToolParam{
			Name:        tool.Name(),
			Description: anthropic.String(tool.Description()),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties:  properties,
				ExtraFields: extraProps,
			},
		}

		// Print the anthropicTool json
		b, err := json.Marshal(anthropicTool)
		if err != nil {
			utils.Logger.Error("Failed to marshal anthropicTool", zap.Error(err))
			continue
		}
		utils.Logger.Debug("Anthropic Tool", zap.String("tool", string(b)))

		anthropicTools = append(anthropicTools, anthropicTool)
	}
	return anthropicTools
}

func (ac *AnthropicConnector) Query(ctx context.Context, qParams *QueryParams) (string, error) {
	ac.logger.Sugar().Debugw("Query", "model", ac.modelID)

	msgParams := anthropic.MessageNewParams{
		MaxTokens: int64(qParams.MaxTokens),
		System: []anthropic.TextBlockParam{
			{Text: *qParams.SysPrompt},
		},
		Messages: []anthropic.MessageParam{{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{{
				OfRequestTextBlock: &anthropic.TextBlockParam{Text: *qParams.UserPrompt},
			}},
		}},
		Model: ac.modelID,
	}

	// If stream, then we use the streaming API and leave this function
	if qParams.Stream {
		return ac.queryAnthropicStream(ctx, msgParams)
	}

	message, err := ac.client.Messages.New(ctx, msgParams)
	if err != nil {
		return "", err
	}

	var outText string
	for _, block := range message.Content {
		txtBlock := block.AsResponseTextBlock()
		outText += txtBlock.Text
	}

	return outText, nil
}

func (ac *AnthropicConnector) QueryWithTool(ctx context.Context, qParams *QueryParams, execTools map[string]tools.Tool) (LlmResponseWithTools, error) {
	logger := *utils.GetLogger()
	logger.Sugar().Debugw("Query with tool", "model", ac.modelID)

	response := LlmResponseWithTools{}

	toolParams := convertToolsToAnthropic(execTools)

	tools := make([]anthropic.ToolUnionParam, len(toolParams))
	for i, toolParam := range toolParams {
		tools[i] = anthropic.ToolUnionParam{OfTool: &toolParam}
	}

	message, err := ac.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     ac.modelID,
		System:    []anthropic.TextBlockParam{{Text: *qParams.SysPrompt}},
		MaxTokens: int64(qParams.MaxTokens),
		Messages: []anthropic.MessageParam{{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{{
				OfRequestTextBlock: &anthropic.TextBlockParam{Text: *qParams.UserPrompt},
			}},
		}},
		Tools: tools,
	})

	if err != nil {
		return response, fmt.Errorf("failed to request Anthropic client: %v", err)
	}

	// Iterate over all blocks in the message
	for _, block := range message.Content {
		switch variant := block.AsAny().(type) {
		case anthropic.TextBlock:
			response.Response += block.Text + "\n"

		case anthropic.ToolUseBlock:
			var input map[string]any
			err := json.Unmarshal([]byte(variant.JSON.Input.Raw()), &input)
			if err != nil {
				ac.logger.Sugar().Errorw("Failed to unmarshal input", "input", variant.JSON.Input.Raw(), "error", err)
				continue
			}
			response.ToolUse = true
			response.ToolName = variant.Name
			response.ToolInput = input
		}

	}
	ac.logger.Sugar().Debugw("Response", "toolResponse", response)
	return response, nil
}
