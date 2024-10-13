package connector

import (
	"context"
	"errors"
	"fmt"
	"log"

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

	ClaudeHaiku BedrockModelID = "anthropic.claude-3-haiku-20240307-v1:0"
)

var (
	ErrBedrockForbidden = fmt.Errorf("bedrock - forbidden")
)

type BedrockConnector struct {
	client  *bedrockruntime.Client
	modelID BedrockModelID
	logger  zap.Logger

	execTools map[string]tools.Tool
	toolSpecs []types.ToolSpecification
}

func NewBedrockConnector(modelID *BedrockModelID, execTools map[string]tools.Tool) *BedrockConnector {
	logger := *utils.GetLogger()
	logger.Debug("NewBedrockConnector")
	// sdkConfig, err := cloud.NewAwsConfigWithSSO(context.Background(), "dev")
	// sdkConfig, err := cloud.NewDefaultAwsConfig(context.Background())
	sdkConfig, err := cloud.NewAwsConfig(context.Background(), config.WithRegion("us-east-1"))

	// Define the input schema as a map
	var toolSpecs []types.ToolSpecification
	for _, tool := range execTools {
		toolSpecs = append(toolSpecs, types.ToolSpecification{
			Name:        aws.String(tool.Name()),
			Description: aws.String(tool.Description()),
			InputSchema: &types.ToolInputSchemaMemberJson{
				Value: document.NewLazyDocument(tool.InputSchema()),
			},
		})
	}

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
		client:    client,
		modelID:   *modelID,
		execTools: execTools,
		toolSpecs: toolSpecs,
		logger:    logger,
	}
}

func (bc *BedrockConnector) queryBedrock(
	ctx context.Context, userPrompt *string, systemPrompt *string, toolConfig *types.ToolConfiguration,
) (*bedrockruntime.ConverseOutput, error) {
	systemPromptContent := []types.SystemContentBlock{
		&types.SystemContentBlockMemberText{Value: *systemPrompt},
	}
	userPromptContent := []types.ContentBlock{
		&types.ContentBlockMemberText{Value: *userPrompt},
	}

	converseInput := &bedrockruntime.ConverseInput{
		ModelId: (*string)(&bc.modelID),
		System:  systemPromptContent,
		Messages: []types.Message{
			{Role: "user", Content: userPromptContent},
		},
	}

	if toolConfig != nil {
		converseInput.ToolConfig = toolConfig
	}

	converseOutput, err := bc.client.Converse(ctx, converseInput)
	if err != nil {
		var re *awshttp.ResponseError
		if errors.As(err, &re) {
			log.Printf("requestID: %s, error: %v", re.ServiceRequestID(), re.Unwrap())
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

	if _, ok := converseOutput.Output.(*types.ConverseOutputMemberMessage); !ok {
		return nil, fmt.Errorf("model %s returned unknown response type", bc.modelID)
	}

	return converseOutput, nil
}

func (bc *BedrockConnector) Query(userPrompt *string, systemPrompt *string) (string, error) {
	bc.logger.Sugar().Debugw("Query", "model", bc.modelID)

	converseOutput, err := bc.queryBedrock(context.Background(), userPrompt, systemPrompt, nil)
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

func (bc *BedrockConnector) QueryWithTool(userPrompt *string, systemPrompt *string) (string, error) {
	bc.logger.Sugar().Debugw("Query with tool", "model", bc.modelID)

	var tools []types.Tool
	for _, toolSpec := range bc.toolSpecs {
		tools = append(tools, &types.ToolMemberToolSpec{
			Value: toolSpec,
		})
	}

	toolConfig := types.ToolConfiguration{
		Tools: tools,
		// TODO: Add tool choice
	}

	converseOutput, err := bc.queryBedrock(context.Background(), userPrompt, systemPrompt, &toolConfig)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}

	if converseOutput.StopReason == ToolUse {
		outputMsg := converseOutput.Output.(*types.ConverseOutputMemberMessage).Value

		// Check whether there's any tool used in the output. If so, execute the tool.
		output, err := bc.findAndUseTool(outputMsg)

		if err != nil {
			return "", fmt.Errorf("failed to find and use tool: %v", err)
		}

		return output, nil
	}

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

// findAndUseTool finds the tool use in the output message and executes the tool.
// If the tool is not supported then it returns an error.
// For supported tools, it executes `tool.RunSchema` method of the tool and returns the command to be executed.
//
// TODO: Only executes a single, first tool. Should be able to execute multiple tools.
func (bc *BedrockConnector) findAndUseTool(outputMessage types.Message) (string, error) {

	// First find the content that actully has the tool use
	for blockIdx, content := range outputMessage.Content {
		bc.logger.Sugar().Debugw("Content block", "blockIdx", blockIdx, "content", content)

		// Only interested in ToolUse content
		if _, ok := content.(*types.ContentBlockMemberToolUse); !ok {
			continue
		}

		var toolUse types.ToolUseBlock = content.(*types.ContentBlockMemberToolUse).Value

		// Check if the tool is supported
		execTool := bc.execTools[*toolUse.Name]

		if execTool == nil {
			return "", fmt.Errorf("tool %s is not supported", *toolUse.Name)
		}

		var inputMap map[string]interface{}
		err := toolUse.Input.UnmarshalSmithyDocument(&inputMap)
		if err != nil {
			return "", fmt.Errorf("failed to unmarshal tool input: %v", err)
		}
		bc.logger.Sugar().Infow("Tool use", "tool", *toolUse.Name, "input", inputMap)

		// Execute the tool and return the output
		return execTool.RunSchema(inputMap)
	}

	return "", fmt.Errorf("no tool use found in the output")
}
