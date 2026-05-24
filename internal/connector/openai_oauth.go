package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/tools"
	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
	"go.uber.org/zap"
)

func convertToolsToResponses(execTools map[string]tools.Tool) []responses.ToolUnionParam {
	toolSpecs := make([]responses.ToolUnionParam, 0, len(execTools))
	for _, tool := range execTools {
		toolSpecs = append(toolSpecs, responses.ToolParamOfFunction(tool.Name(), tool.InputSchema(), false))
	}
	return toolSpecs
}

func roleToResponses(role string) responses.EasyInputMessageRole {
	switch role {
	case "assistant":
		return responses.EasyInputMessageRoleAssistant
	default:
		return responses.EasyInputMessageRoleUser
	}
}

func buildResponsesParams(modelID string, qParams *QueryParams) responses.ResponseNewParams {
	input := responses.ResponseInputParam{}
	for _, msg := range qParams.Messages {
		input = append(input, responses.ResponseInputItemParamOfMessage(msg.Content, roleToResponses(msg.Role)))
	}
	if qParams.UserPrompt != nil {
		input = append(input, responses.ResponseInputItemParamOfMessage(*qParams.UserPrompt, responses.EasyInputMessageRoleUser))
	}

	params := responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
		Model:        shared.ResponsesModel(modelID),
		Instructions: openai.String(*qParams.SysPrompt),
	}
	if qParams.MaxTokens > 0 {
		params.MaxOutputTokens = openai.Int(int64(qParams.MaxTokens))
	}
	return params
}

func (oc *OpenAIConnector) streamOAuthQuery(ctx context.Context, qParams *QueryParams, params responses.ResponseNewParams) (*string, error) {
	var mdRenderer *MarkdownStreamRenderer
	if qParams.OnStream == nil {
		renderer, err := NewMarkdownStreamRenderer()
		if err != nil {
			oc.logger.Warn("Failed to create markdown renderer, falling back to plain text", zap.Error(err))
		} else {
			mdRenderer = renderer
		}
	}

	stream := oc.client.Responses.NewStreaming(ctx, params)
	var streamedText strings.Builder
	var completedText string

	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "response.output_text.delta":
			delta := event.AsResponseOutputTextDelta().Delta
			streamedText.WriteString(delta)
			if qParams.OnStream != nil {
				if err := qParams.OnStream(delta); err != nil {
					return nil, err
				}
			} else if mdRenderer != nil {
				mdRenderer.ProcessChunk(delta)
			} else {
				print(delta)
			}
		case "response.completed":
			completedText = event.AsResponseCompleted().Response.OutputText()
		}
	}

	if err := stream.Err(); err != nil {
		return nil, err
	}

	if mdRenderer != nil {
		mdRenderer.Flush()
	}

	if completedText != "" {
		return &completedText, nil
	}

	text := streamedText.String()
	return &text, nil
}

func (oc *OpenAIConnector) queryOAuth(ctx context.Context, qParams *QueryParams) (string, error) {
	params := buildResponsesParams(oc.modelID, qParams)
	if qParams.Stream {
		result, err := oc.streamOAuthQuery(ctx, qParams, params)
		if err != nil {
			return "", err
		}
		if result == nil {
			return "", nil
		}
		return *result, nil
	}

	response, err := oc.client.Responses.New(ctx, params)
	if err != nil {
		return "", err
	}

	return response.OutputText(), nil
}

func (oc *OpenAIConnector) queryWithToolOAuth(ctx context.Context, qParams *QueryParams, tools map[string]tools.Tool) (LlmResponseWithTools, error) {
	response := LlmResponseWithTools{}
	params := buildResponsesParams(oc.modelID, qParams)
	params.Tools = convertToolsToResponses(tools)
	params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
		OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsAuto),
	}
	params.ParallelToolCalls = openai.Bool(true)

	result, err := oc.client.Responses.New(ctx, params)
	if err != nil {
		return response, err
	}

	response.Response = result.OutputText()
	for _, item := range result.Output {
		if item.Type != "function_call" {
			continue
		}
		if item.Name == "" {
			return response, fmt.Errorf("tool call name is empty")
		}
		if item.Arguments == "" {
			return response, fmt.Errorf("tool call arguments are empty")
		}

		var args map[string]any
		if err := json.Unmarshal([]byte(item.Arguments), &args); err != nil {
			return response, fmt.Errorf("failed to parse tool call arguments: %w", err)
		}

		response.ToolUse = true
		response.ToolName = item.Name
		response.ToolInput = args
	}

	return response, nil
}
