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
	ModelPricesOpenai = map[string]map[string]float64{
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

	execTools map[string]tools.Tool
	toolSpecs []openai.ChatCompletionToolParam
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
			Type: openai.F(openai.ChatCompletionToolTypeFunction),
			Function: openai.F(openai.FunctionDefinitionParam{
				Name:        openai.F(tool.Name()),
				Description: openai.F(tool.Description()),
				Parameters:  openai.F(openai.FunctionParameters(inputSchema)),
			}),
		})
	}

	return toolSpecs
}

func NewOpenAIConnector(modelID *string, execTools map[string]tools.Tool) *OpenAIConnector {
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
		client:    client,
		logger:    logger,
		modelID:   *modelID,
		token:     token,
		execTools: execTools,
		toolSpecs: convertToolsToOpenAI(execTools),
	}

	return connector
}

func (oc *OpenAIConnector) Query(userPrompt *string, systemPrompt *string) (string, error) {

	params := openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(*systemPrompt),
			openai.UserMessage(*userPrompt),
		}),
		Model: openai.F(oc.modelID),
	}

	completion, err := oc.client.Chat.Completions.New(context.TODO(), params)
	if err != nil {
		return "", err
	}

	if completion != nil {
		price := computePriceOpenai(oc.modelID, &completion.Usage)
		oc.logger.Sugar().Debugw("Usage", "usage", completion.Usage, "price", price)
	}

	return completion.Choices[0].Message.Content, nil
}

func (oc *OpenAIConnector) QueryWithTool(userPrompt *string, systemPrompt *string) (string, error) {
	ctx := context.TODO()

	client := openai.NewClient(
		option.WithAPIKey(oc.token),
	)

	params := openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(*systemPrompt),
			openai.UserMessage(*userPrompt),
		}),
		Tools: openai.F(oc.toolSpecs),
		Model: openai.F(oc.modelID),
	}

	completion, _ := client.Chat.Completions.New(ctx, params)

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
	return oc.findAndUseTool(completion.Choices)

}

func (oc *OpenAIConnector) findAndUseTool(choices []openai.ChatCompletionChoice) (string, error) {
	for _, choice := range choices {
		message := choice.Message
		for _, toolCall := range message.ToolCalls {

			execTool, exist := oc.execTools[toolCall.Function.Name]
			if !exist {
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
