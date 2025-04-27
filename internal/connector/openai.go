package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"go.uber.org/zap"
)

const (
	OpenaiProvider = "openai"
)

var (
	// https://openai.com/api/pricing/
	ModelPricesOpenai = map[openai.ChatModel]map[string]float64{
		openai.ChatModelGPT4o: {
			"input":  0.00025,
			"output": 0.01000,
		},
		openai.ChatModelGPT4o2024_08_06: {
			"input":  0.00250,
			"output": 0.01000,
		},
		openai.ChatModelGPT4o2024_05_13: {
			"input":  0.00500,
			"output": 0.01500,
		},
		openai.ChatModelGPT4oMini: {
			"input":  0.000150,
			"output": 0.000600,
		},
		openai.ChatModelGPT4oMini2024_07_18: {
			"input":  0.000150,
			"output": 0.000600,
		},
		openai.ChatModelO1Preview: {
			"input":  0.015,
			"output": 0.060,
		},
		openai.ChatModelO1Preview2024_09_12: {
			"input":  0.015,
			"output": 0.060,
		},
	}
)

type OpenAIConnector struct {
	client  *openai.Client
	logger  zap.Logger
	modelID string
	token   string
}

func computePriceOpenai(modelId string, usage *openai.CompletionUsage) *LLMPrice {
	prices, exists := ModelPricesOpenai[modelId]
	if !exists {
		return nil
	}
	ip := prices["input"] * float64(usage.PromptTokens) / 1000
	op := prices["output"] * float64(usage.CompletionTokens) / 1000
	return &LLMPrice{
		InputPrice:  ip,
		OutputPrice: op,
		TotalPrice:  ip + op,
	}
}

func convertToolsToOpenAI(execTools map[string]tools.Tool) []openai.ChatCompletionToolParam {
	var toolSpecs []openai.ChatCompletionToolParam
	for _, tool := range execTools {
		inputSchema := tool.InputSchema()

		toolSpecs = append(toolSpecs, openai.ChatCompletionToolParam{
			// Type: openai.ChatCompletionToolTypeFunction,
			Function: openai.FunctionDefinitionParam{
				Name:        tool.Name(),
				Description: openai.Opt(tool.Description()),
				Parameters:  openai.FunctionParameters(inputSchema),
			},
		})
	}
	return toolSpecs
}

func NewOpenAIConnector(modelID *string) *OpenAIConnector {
	logger := *utils.GetLogger()
	logger.Debug("NewOpenAiConnector")

	if modelID == nil || *modelID == "" {
		model := openai.ChatModelGPT4oMini
		modelID = &model
	}
	token := os.Getenv("OPENAI_API_KEY")
	client := openai.NewClient(
		option.WithAPIKey(token),
	)

	connector := &OpenAIConnector{
		client:  &client,
		logger:  logger,
		modelID: *modelID,
		token:   token,
	}

	return connector
}

func (oc *OpenAIConnector) streamQuery(ctx context.Context, params openai.ChatCompletionNewParams) (*string, error) {

	stream := oc.client.Chat.Completions.NewStreaming(ctx, params)

	acc := openai.ChatCompletionAccumulator{}

	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		// if using tool calls
		if tool, ok := acc.JustFinishedToolCall(); ok {
			println("Tool call stream finished:", tool.Index, tool.Name, tool.Arguments)
		}

		if refusal, ok := acc.JustFinishedRefusal(); ok {
			println("Refusal stream finished:", refusal)
		}

		// it's best to use chunks after handling JustFinished events
		if len(chunk.Choices) > 0 {
			print(chunk.Choices[0].Delta.Content)
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	return &acc.Choices[0].Message.Content, nil
}

func (oc *OpenAIConnector) Query(ctx context.Context, qParams *QueryParams) (string, error) {

	oParams := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(*qParams.SysPrompt),
			openai.UserMessage(*qParams.UserPrompt),
		},
		Model: oc.modelID,
	}

	if qParams.Stream {
		s, err := oc.streamQuery(ctx, oParams)
		return *s, err
	}

	completion, err := oc.client.Chat.Completions.New(ctx, oParams)
	if err != nil {
		return "", err
	}

	if completion != nil {
		price := computePriceOpenai(oc.modelID, &completion.Usage)
		oc.logger.Sugar().Debugw("Usage", "usage", completion.Usage, "price", price)
	}

	return completion.Choices[0].Message.Content, nil
}

func (oc *OpenAIConnector) QueryWithTool(ctx context.Context, qParams *QueryParams, tools map[string]tools.Tool) (string, error) {
	client := openai.NewClient(
		option.WithAPIKey(oc.token),
	)

	oParams := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(*qParams.SysPrompt),
			openai.UserMessage(*qParams.UserPrompt),
		},
		Tools: convertToolsToOpenAI(tools),
		Model: oc.modelID,
	}

	completion, _ := client.Chat.Completions.New(ctx, oParams)

	if completion != nil {
		price := computePriceOpenai(oc.modelID, &completion.Usage)
		oc.logger.Sugar().Debugw("Usage", "usage", completion.Usage, "price", price)
	}

	// Check if any tools call
	allToolCalls := 0
	for _, choice := range completion.Choices {
		allToolCalls += len(choice.Message.ToolCalls)
	}

	// No tools used
	if allToolCalls == 0 {
		return completion.Choices[0].Message.Content, nil
	}

	// Tool used
	return oc.findAndUseTool(completion.Choices, tools)

}

func (oc *OpenAIConnector) findAndUseTool(choices []openai.ChatCompletionChoice, execTools map[string]tools.Tool) (string, error) {
	for _, choice := range choices {
		message := choice.Message
		for _, toolCall := range message.ToolCalls {

			// Check if the tool call is valid
			if toolCall.Function.Name == "" {
				return "", fmt.Errorf("tool call name is empty")
			}
			if toolCall.Function.Arguments == "" {
				return "", fmt.Errorf("tool call arguments are empty")
			}
			execTool, exist := execTools[toolCall.Function.Name]
			if !exist {
				// Check if the tool call is valid
				return "", fmt.Errorf("tool %s not found", toolCall.Function.Name)
			}

			var args map[string]interface{}
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
				panic(err)
			}

			output, err := execTool.RunSchema(args)
			if err != nil {
				return "", err
			}

			return output, nil
		}
	}

	return "", nil
}
