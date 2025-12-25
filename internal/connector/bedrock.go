package connector

import (
	"context"
	"errors"
	"fmt"
	"os"

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

// convertToolsToBedrock converts a map of tools.Tool to AWS Bedrock's types.Tool format.
// It extracts the name, description, and input schema from each tool and formats them
// according to AWS Bedrock API requirements.
//
// Parameters:
//   - tools: A map of string to tools.Tool representing the available tools
//
// Returns:
//   - []types.Tool: A slice of Bedrock-compatible tool specifications
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

func NewBedrockConnector(modelID *BedrockModelID) *BedrockConnector {
	logger := *utils.GetLogger()
	logger.Debug("NewBedrockConnector")
	// sdkConfig, err := cloud.NewAwsConfigWithSSO(context.Background(), "dev")
	// sdkConfig, err := cloud.NewDefaultAwsConfig(context.Background())

	// Load the AWS region from environment variable or use default
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		awsRegion = DefaultAwsRegion
	}

	sdkConfig, err := cloud.NewAwsConfig(context.Background(), config.WithRegion(awsRegion))

	if err != nil {
		fmt.Println("Couldn't load default configuration. Have you set up your AWS account?")
		fmt.Println(err)
		return nil
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
	}
}

func (bc *BedrockConnector) queryBedrock(
	ctx context.Context, qParams *QueryParams, toolConfig *types.ToolConfiguration,
) (*bedrockruntime.ConverseOutput, error) {

	// Build messages list from history + current user prompt
	messages := bc.buildMessages(qParams)

	converseInput := &bedrockruntime.ConverseInput{
		ModelId: (*string)(&bc.modelID),
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
			return nil, fmt.Errorf("failed to send request: %v", err)
		}

		if re.ResponseError.HTTPStatusCode() == 403 {
			return nil, ErrBedrockForbidden
		}

		return nil, fmt.Errorf("failed to send request: %v", err)
	}

	// Only text output is supported
	if converseOutput.Output == nil {
		return nil, fmt.Errorf("model %s returned response is nil", bc.modelID)
	}
	if converseOutput.Usage != nil {
		usage := converseOutput.Usage
		price := computePriceBedrock(bc.modelID, &BedrockUsage{
			InputTokens:  *usage.InputTokens,
			OutputTokens: *usage.OutputTokens,
			TotalTokens:  *usage.TotalTokens,
		})

		bc.logger.Sugar().Debugw("Usage", "usage", converseOutput.Usage, "price", price)
	}

	if _, ok := converseOutput.Output.(*types.ConverseOutputMemberMessage); !ok {
		return nil, fmt.Errorf("model %s returned unknown response type", bc.modelID)
	}

	return converseOutput, nil
}

func (bc *BedrockConnector) queryBedrockStream(
	ctx context.Context, qParams *QueryParams, toolConfig *types.ToolConfiguration,
) (string, error) {
	// Create markdown renderer for streaming
	mdRenderer, err := NewMarkdownStreamRenderer()
	if err != nil {
		// Fallback to simple streaming if renderer fails
		bc.logger.Warn("Failed to create markdown renderer, falling back to plain text", zap.Error(err))
		mdRenderer = nil
	}

	// Build messages list from history + current user prompt
	messages := bc.buildMessages(qParams)

	converseInput := &bedrockruntime.ConverseStreamInput{
		ModelId: (*string)(&bc.modelID),
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

		if re.ResponseError.HTTPStatusCode() == 403 {
			return "", ErrBedrockForbidden
		}

		return "", fmt.Errorf("failed to send request: %v", err)
	}

	var acc string

	stream := converseOutput.GetStream()
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
			fmt.Println()

		case *types.ConverseStreamOutputMemberContentBlockStop:
			stop := event.Value
			bc.logger.Debug("ContentBlockStart", zap.Any("stop", stop))
			fmt.Println()

		case *types.ConverseStreamOutputMemberContentBlockDelta:
			chunk, isText := event.Value.Delta.(*types.ContentBlockDeltaMemberText)
			if !isText {
				continue
			}
			if mdRenderer != nil {
				mdRenderer.ProcessChunk(chunk.Value)
			} else {
				fmt.Print(chunk.Value)
			}
			acc += chunk.Value

		case *types.ConverseStreamOutputMemberMetadata:
			usage := event.Value.Usage
			price := computePriceBedrock(bc.modelID, &BedrockUsage{
				InputTokens:  *usage.InputTokens,
				OutputTokens: *usage.OutputTokens,
				TotalTokens:  *usage.TotalTokens,
			})

			bc.logger.Sugar().Debugw("Usage", "usage", usage, "price", price)

		default:
			bc.logger.Warn("union is nil or unknown type", zap.Any("event", event))
		}
	}

	// Flush any remaining content
	if mdRenderer != nil {
		mdRenderer.Flush()
	}

	return acc, nil
}

func (bc *BedrockConnector) Query(ctx context.Context, qParams *QueryParams) (string, error) {
	bc.logger.Sugar().Debugw("Query", "model", bc.modelID)

	if qParams.Stream {
		return bc.queryBedrockStream(ctx, qParams, nil)
	}

	converseOutput, err := bc.queryBedrock(context.Background(), qParams, nil)
	if err != nil {
		return "", err
	}

	// If converseOutput.stopReason ==
	var response string
	var msgResponse types.Message

	union := converseOutput.Output
	if union == nil {
		return "", fmt.Errorf("model %s returned response is nil", bc.modelID)
	}

	if _, ok := union.(*types.ConverseOutputMemberMessage); !ok {
		return "", fmt.Errorf("model %s returned unknown response type", bc.modelID)
	}
	msgResponse = union.(*types.ConverseOutputMemberMessage).Value // Value is types.Message
	response = msgResponse.Content[0].(*types.ContentBlockMemberText).Value

	return response, nil
}

func (bc *BedrockConnector) QueryWithTool(ctx context.Context, qParams *QueryParams, execTools map[string]tools.Tool) (LlmResponseWithTools, error) {
	bc.logger.Sugar().Debugw("Query with tool", "model", bc.modelID)
	response := LlmResponseWithTools{}

	toolConfig := types.ToolConfiguration{
		Tools:      convertToolsToBedrock(execTools),
		ToolChoice: &types.ToolChoiceMemberAuto{},
	}

	//
	converseOutput, err := bc.queryBedrock(context.Background(), qParams, &toolConfig)
	if err != nil {
		return response, fmt.Errorf("failed to send request: %v", err)
	}

	if converseOutput.StopReason == ToolUse {
		bc.logger.Sugar().Debugw("Tool use", "stopReason", converseOutput.StopReason)
	}

	contents := converseOutput.Output.(*types.ConverseOutputMemberMessage).Value.Content
	for blockIdx, content := range contents {
		bc.logger.Sugar().Debugw("Content block", "blockIdx", blockIdx, "content", content)
		if contentBlockMemberText, ok := content.(*types.ContentBlockMemberText); ok {
			response.Response += contentBlockMemberText.Value + "\n"
		}

		if contentBlockMemberText, ok := content.(*types.ContentBlockMemberToolUse); ok {
			bc.logger.Sugar().Debugw("Tool use", "blockIdx", blockIdx, "content", content)
			contentToolUse := contentBlockMemberText.Value
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
