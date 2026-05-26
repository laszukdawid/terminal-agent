package connector

import (
	"context"
	"encoding/json"
	"errors"
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
		Store:        openai.Bool(false),
	}
	return params
}

func wrapOpenAIAPIError(err error) error {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	if apiErr.Message != "" {
		return err
	}

	raw := strings.TrimSpace(apiErr.RawJSON())
	if raw != "" {
		return fmt.Errorf("%w: %s", err, raw)
	}

	responseDump := strings.TrimSpace(string(apiErr.DumpResponse(true)))
	if responseDump != "" {
		return fmt.Errorf("%w: %s", err, responseDump)
	}

	return err
}

func (oc *OpenAIConnector) streamOAuthResponse(ctx context.Context, qParams *QueryParams, params responses.ResponseNewParams) (*responses.Response, *string, error) {
	var mdRenderer *MarkdownStreamRenderer
	if qParams.Stream && qParams.OnStream == nil {
		renderer, err := NewMarkdownStreamRenderer()
		if err != nil {
			oc.logger.Warn("Failed to create markdown renderer, falling back to plain text", zap.Error(err))
		} else {
			mdRenderer = renderer
		}
	}

	stream := oc.client.Responses.NewStreaming(ctx, params)
	var streamedText strings.Builder
	var completedResponse *responses.Response

	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "response.output_text.delta":
			delta := event.AsResponseOutputTextDelta().Delta
			streamedText.WriteString(delta)
			if !qParams.Stream {
				continue
			}
			if qParams.OnStream != nil {
				if err := qParams.OnStream(delta); err != nil {
					return nil, nil, err
				}
			} else if mdRenderer != nil {
				mdRenderer.ProcessChunk(delta)
			} else {
				print(delta)
			}
		case "response.completed":
			response := event.AsResponseCompleted().Response
			completedResponse = &response
		}
	}

	if err := stream.Err(); err != nil {
		return nil, nil, wrapOpenAIAPIError(err)
	}

	if mdRenderer != nil {
		mdRenderer.Flush()
	}

	if completedResponse != nil {
		text := completedResponse.OutputText()
		if text == "" && streamedText.Len() > 0 {
			text = streamedText.String()
		}
		return completedResponse, &text, nil
	}

	text := streamedText.String()
	return nil, &text, nil
}

func (oc *OpenAIConnector) queryOAuth(ctx context.Context, qParams *QueryParams) (string, error) {
	params := buildResponsesParams(oc.modelID, qParams)
	_, result, err := oc.streamOAuthResponse(ctx, qParams, params)
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", nil
	}
	return *result, nil
}

func (oc *OpenAIConnector) queryWithToolOAuth(ctx context.Context, qParams *QueryParams, tools map[string]tools.Tool) (LlmResponseWithTools, error) {
	response := LlmResponseWithTools{}
	params := buildResponsesParams(oc.modelID, qParams)
	params.Tools = convertToolsToResponses(tools)
	params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
		OfToolChoiceMode: openai.Opt(responses.ToolChoiceOptionsAuto),
	}
	params.ParallelToolCalls = openai.Bool(true)

	nonStreamingParams := *qParams
	nonStreamingParams.Stream = false
	nonStreamingParams.OnStream = nil

	result, _, err := oc.streamOAuthResponse(ctx, &nonStreamingParams, params)
	if err != nil {
		return response, err
	}
	if result == nil {
		return response, fmt.Errorf("no response received from OAuth responses stream")
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
