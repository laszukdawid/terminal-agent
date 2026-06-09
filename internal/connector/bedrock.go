package connector

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/laszukdawid/terminal-agent/internal/cloud"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
)

type BedrockModelID string

const (
	BedrockProvider = "bedrock"
	ToolUse         = "tool_use"

	DefaultAwsRegion = "us-east-1"

	// Supported models https://docs.aws.amazon.com/bedrock/latest/userguide/conversation-inference-supported-models-features.html
	ClaudeHaiku      BedrockModelID = "anthropic.claude-3-haiku-20240307-v1:0"
	JambaMini        BedrockModelID = "ai21.jamba-1-5-mini-v1:0"
	MistralSmall2402 BedrockModelID = "mistral.mistral-small-2402-v1:0"
	MistralLarge2402 BedrockModelID = "mistral.mistral-large-2402-v1:0"
)

var (
	ErrBedrockForbidden = fmt.Errorf("bedrock - forbidden")

	// All prices are per 1k tokens
	ModelPricesBedrock = map[BedrockModelID]map[string]float64{
		ClaudeHaiku: {
			"input":  0.00025,
			"output": 0.00125,
		},
		JambaMini: {
			"input":  0.0002,
			"output": 0.0004,
		},
		MistralSmall2402: {
			"input":  0.001,
			"output": 0.003,
		},
		MistralLarge2402: {
			"input":  0.004,
			"output": 0.012,
		},
	}
)

/*
Pricing from https://aws.amazon.com/bedrock/pricing/

Model                  | Input per 1k tokens | Output per 1k tokens
-----------------------|---------------------|----------------------
Haiku                  | $0.00025            | $0.00125
Llama 3.2 (1B)         | $0.0001             | $0.0001
Llama 3.2 (3B)         | $0.00015            | $0.00015
Llama 3 (8B)           | $0.0003             | $0.0006
Mistral Small (24.02)  | $0.001              | $0.003
Mistral Large (24.02)  | $0.004              | $0.012
Jamba 1.5 Mini         | $0.0002             | $0.0004
*/

type BedrockConnector struct {
	client  *bedrockruntime.Client
	modelID BedrockModelID
	logger  zap.Logger
}

type BedrockSettings struct {
	Profile string
	Region  string
}

type BedrockConfigProvider interface {
	GetBedrockProfile() string
	GetBedrockRegion() string
}

func computePriceBedrock(modelId BedrockModelID, usage *BedrockUsage) *LLMPrice {
	prices, exists := ModelPricesBedrock[modelId]
	if !exists {
		return nil
	}
	ip := prices["input"] * float64(usage.InputTokens) / 1000
	op := prices["output"] * float64(usage.OutputTokens) / 1000
	return &LLMPrice{
		InputPrice:  ip,
		OutputPrice: op,
		TotalPrice:  ip + op,
	}
}

func bedrockInferenceConfig(qParams *QueryParams) *types.InferenceConfiguration {
	if qParams.MaxTokens <= 0 {
		return nil
	}

	maxTokens := int32(qParams.MaxTokens)
	return &types.InferenceConfiguration{MaxTokens: &maxTokens}
}

func bedrockUsageFromOutput(usage *types.TokenUsage) *BedrockUsage {
	if usage == nil {
		return nil
	}

	var bedrockUsage BedrockUsage
	if usage.InputTokens != nil {
		bedrockUsage.InputTokens = *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		bedrockUsage.OutputTokens = *usage.OutputTokens
	}
	if usage.TotalTokens != nil {
		bedrockUsage.TotalTokens = *usage.TotalTokens
	}
	return &bedrockUsage
}

func textFromBedrockMessage(modelID BedrockModelID, message types.Message) (string, error) {
	var response string
	for _, content := range message.Content {
		if text, ok := content.(*types.ContentBlockMemberText); ok {
			response += text.Value
		}
	}
	if response == "" {
		return "", fmt.Errorf("model %s returned no text content", modelID)
	}
	return response, nil
}

func bedrockSettingsFromConfig(cfg any) BedrockSettings {
	provider, ok := cfg.(BedrockConfigProvider)
	if !ok {
		return BedrockSettings{}
	}
	return BedrockSettings{
		Profile: strings.TrimSpace(provider.GetBedrockProfile()),
		Region:  strings.TrimSpace(provider.GetBedrockRegion()),
	}
}

func bedrockAWSConfigOptions(settings BedrockSettings) []func(*config.LoadOptions) error {
	var opts []func(*config.LoadOptions) error
	if settings.Profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(settings.Profile))
	}
	if settings.Region != "" {
		opts = append(opts, config.WithRegion(settings.Region))
	}
	return opts
}

// buildMessages constructs the messages slice from history + current user prompt
func (bc *BedrockConnector) buildMessages(qParams *QueryParams) []types.Message {
	var messages []types.Message

	// Add conversation history
	for _, msg := range qParams.Messages {
		messages = append(messages, types.Message{
			Role:    types.ConversationRole(msg.Role),
			Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: msg.Content}},
		})
	}

	// Add current user prompt
	if qParams.UserPrompt != nil && *qParams.UserPrompt != "" {
		messages = append(messages, types.Message{
			Role:    "user",
			Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: *qParams.UserPrompt}},
		})
	}

	return messages
}

// convertToolsToBedrock converts tool definitions to Bedrock tool specifications.
func convertToolsToBedrock(tools map[string]tools.Tool) []types.Tool {
	// Define the input schema as a map
	var bedrockTools []types.Tool
	for _, tool := range tools {
		bedrockTool := &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        aws.String(tool.Name()),
				Description: aws.String(tool.Description()),
				InputSchema: &types.ToolInputSchemaMemberJson{
					Value: document.NewLazyDocument(tool.InputSchema()),
				},
			},
		}
		bedrockTools = append(bedrockTools, bedrockTool)
	}
	return bedrockTools
}

func NewBedrockConnector(modelID *BedrockModelID, cfg any) (*BedrockConnector, error) {
	logger := *utils.GetLogger()
	logger.Debug("NewBedrockConnector")

	settings := bedrockSettingsFromConfig(cfg)
	sdkConfig, err := cloud.NewAwsConfig(context.Background(), bedrockAWSConfigOptions(settings)...)

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS configuration for Bedrock: %w", err)
	}
	if sdkConfig.Region == "" {
		sdkConfig.Region = DefaultAwsRegion
	}

	if modelID == nil || *modelID == "" {
		model := ClaudeHaiku
		modelID = &model
	}

	client := bedrockruntime.NewFromConfig(sdkConfig)

	return &BedrockConnector{
		client:  client,
		modelID: *modelID,
		logger:  logger,
	}, nil
}

func (bc *BedrockConnector) queryBedrock(
	ctx context.Context, qParams *QueryParams, toolConfig *types.ToolConfiguration,
) (*bedrockruntime.ConverseOutput, error) {

	// Build messages list from history + current user prompt
	messages := bc.buildMessages(qParams)

	converseInput := &bedrockruntime.ConverseInput{
		ModelId:         (*string)(&bc.modelID),
		InferenceConfig: bedrockInferenceConfig(qParams),
		System: []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: *qParams.SysPrompt},
		},
		Messages: messages,
	}

	if toolConfig != nil {
		converseInput.ToolConfig = toolConfig
	}

	converseOutput, err := bc.client.Converse(ctx, converseInput)
	if err != nil {
		var re *awshttp.ResponseError
		if errors.As(err, &re) {
			bc.logger.Sugar().Errorf("requestID: %s, error: %v", re.ServiceRequestID(), re.Unwrap())
		}

		if re == nil || re.ResponseError == nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}

		if re.ResponseError.HTTPStatusCode() == 403 {
			return nil, ErrBedrockForbidden
		}

		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Only text output is supported
	if converseOutput.Output == nil {
		return nil, fmt.Errorf("model %s returned response is nil", bc.modelID)
	}
	if usage := bedrockUsageFromOutput(converseOutput.Usage); usage != nil {
		price := computePriceBedrock(bc.modelID, usage)

		bc.logger.Sugar().Debugw("Usage", "usage", usage, "price", price)
	}

	if _, ok := converseOutput.Output.(*types.ConverseOutputMemberMessage); !ok {
		return nil, fmt.Errorf("model %s returned unknown response type", bc.modelID)
	}

	return converseOutput, nil
}

func (bc *BedrockConnector) queryBedrockStream(
	ctx context.Context, qParams *QueryParams, toolConfig *types.ToolConfiguration,
) (string, error) {
	var mdRenderer *MarkdownStreamRenderer
	if qParams.OnStream == nil {
		renderer, err := NewMarkdownStreamRenderer()
		if err != nil {
			bc.logger.Warn("Failed to create markdown renderer, falling back to plain text", zap.Error(err))
		} else {
			mdRenderer = renderer
		}
	}

	// Build messages list from history + current user prompt
	messages := bc.buildMessages(qParams)

	converseInput := &bedrockruntime.ConverseStreamInput{
		ModelId:         (*string)(&bc.modelID),
		InferenceConfig: bedrockInferenceConfig(qParams),
		System: []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: *qParams.SysPrompt},
		},
		Messages: messages,
	}

	if toolConfig != nil {
		converseInput.ToolConfig = toolConfig
	}

	converseOutput, err := bc.client.ConverseStream(ctx, converseInput)
	if err != nil {
		var re *awshttp.ResponseError
		if errors.As(err, &re) {
			bc.logger.Sugar().Errorf("requestID: %s, error: %v", re.ServiceRequestID(), re.Unwrap())
		}

		if re == nil || re.ResponseError == nil {
			return "", fmt.Errorf("failed to send request: %w", err)
		}

		if re.ResponseError.HTTPStatusCode() == 403 {
			return "", ErrBedrockForbidden
		}

		return "", fmt.Errorf("failed to send request: %w", err)
	}

	var acc string

	stream := converseOutput.GetStream()
	defer stream.Close()
	var events <-chan types.ConverseStreamOutput = stream.Events()

	for _event := range events {
		switch event := _event.(type) {

		// Message start contains info about "role"
		case *types.ConverseStreamOutputMemberMessageStart:
			v := event.Value
			if v.Role != "assistant" {
				bc.logger.Sugar().Debugw("Weird MessageStart", "role", v.Role)
			}

		// Message stop contains info about "stopReason" and "additionalModelResponseFields"
		case *types.ConverseStreamOutputMemberMessageStop:
			v := event.Value
			bc.logger.Sugar().Debugw("MessageStop",
				"stopReason", v.StopReason, "additionalModelResponseFields", v.AdditionalModelResponseFields)

		case *types.ConverseStreamOutputMemberContentBlockStart:
			start := event.Value.Start
			bc.logger.Debug("ContentBlockStart", zap.Any("start", start))

		case *types.ConverseStreamOutputMemberContentBlockStop:
			stop := event.Value
			bc.logger.Debug("ContentBlockStop", zap.Any("stop", stop))

		case *types.ConverseStreamOutputMemberContentBlockDelta:
			chunk, isText := event.Value.Delta.(*types.ContentBlockDeltaMemberText)
			if !isText {
				continue
			}
			if qParams.OnStream != nil {
				if err := qParams.OnStream(chunk.Value); err != nil {
					return "", err
				}
			} else if mdRenderer != nil {
				mdRenderer.ProcessChunk(chunk.Value)
			} else {
				fmt.Print(chunk.Value)
			}
			acc += chunk.Value

		case *types.ConverseStreamOutputMemberMetadata:
			usage := bedrockUsageFromOutput(event.Value.Usage)
			if usage != nil {
				price := computePriceBedrock(bc.modelID, usage)
				bc.logger.Sugar().Debugw("Usage", "usage", usage, "price", price)
			}

		default:
			bc.logger.Warn("union is nil or unknown type", zap.Any("event", event))
		}
	}

	// Flush any remaining content
	if mdRenderer != nil {
		mdRenderer.Flush()
	}
	if err := stream.Err(); err != nil {
		return "", fmt.Errorf("failed to read response stream: %w", err)
	}

	return acc, nil
}

func (bc *BedrockConnector) Query(ctx context.Context, qParams *QueryParams) (string, error) {
	bc.logger.Sugar().Debugw("Query", "model", bc.modelID)

	if qParams.Stream {
		return bc.queryBedrockStream(ctx, qParams, nil)
	}

	converseOutput, err := bc.queryBedrock(ctx, qParams, nil)
	if err != nil {
		return "", err
	}

	union := converseOutput.Output
	if union == nil {
		return "", fmt.Errorf("model %s returned response is nil", bc.modelID)
	}

	messageOutput, ok := union.(*types.ConverseOutputMemberMessage)
	if !ok {
		return "", fmt.Errorf("model %s returned unknown response type", bc.modelID)
	}

	return textFromBedrockMessage(bc.modelID, messageOutput.Value)
}

func (bc *BedrockConnector) QueryWithTool(ctx context.Context, qParams *QueryParams, execTools map[string]tools.Tool) (LlmResponseWithTools, error) {
	bc.logger.Sugar().Debugw("Query with tool", "model", bc.modelID)
	response := LlmResponseWithTools{}

	toolConfig := types.ToolConfiguration{
		Tools:      convertToolsToBedrock(execTools),
		ToolChoice: &types.ToolChoiceMemberAuto{},
	}

	//
	converseOutput, err := bc.queryBedrock(ctx, qParams, &toolConfig)
	if err != nil {
		return response, fmt.Errorf("failed to send request: %w", err)
	}

	if converseOutput.StopReason == ToolUse {
		bc.logger.Sugar().Debugw("Tool use", "stopReason", converseOutput.StopReason)
	}

	messageOutput, ok := converseOutput.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return response, fmt.Errorf("model %s returned unknown response type", bc.modelID)
	}

	return parseBedrockToolResponse(bc.modelID, messageOutput.Value)
}

func parseBedrockToolResponse(modelID BedrockModelID, message types.Message) (LlmResponseWithTools, error) {
	response := LlmResponseWithTools{}
	contents := message.Content
	for _, content := range contents {
		if contentBlockMemberText, ok := content.(*types.ContentBlockMemberText); ok {
			response.Response += contentBlockMemberText.Value + "\n"
		}

		if contentBlockMemberText, ok := content.(*types.ContentBlockMemberToolUse); ok {
			contentToolUse := contentBlockMemberText.Value
			if contentToolUse.Name == nil || *contentToolUse.Name == "" {
				return response, fmt.Errorf("model %s returned tool use with empty name", modelID)
			}
			if contentToolUse.Input == nil {
				return response, fmt.Errorf("model %s returned tool use with empty input", modelID)
			}
			response.ToolUse = true
			response.ToolName = *contentToolUse.Name

			// Parse tool's input
			var inputMap map[string]any
			err := contentToolUse.Input.UnmarshalSmithyDocument(&inputMap)
			if err != nil {
				return response, fmt.Errorf("failed to unmarshal tool input: %v", err)
			}
			response.ToolInput = inputMap
		}
	}

	return response, nil
}

func (bc *BedrockConnector) SupportsNativeToolCalling() bool {
	return true
}
