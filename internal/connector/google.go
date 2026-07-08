package connector

import (
	"context"
	"fmt"
	"os"

	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
	"google.golang.org/api/option"

	genai "github.com/google/generative-ai-go/genai"
)

const (
	GoogleProvider    = "google"
	Gemini31FlashLite = "gemini-3.1-flash-lite"
)

type GoogleConnector struct {
	client  *genai.Client
	model   *genai.GenerativeModel
	logger  zap.Logger
	modelID string
}

func NewGoogleConnector(modelID *string) (*GoogleConnector, error) {
	logger := *utils.GetLogger()
	logger.Debug("NewGoogleConnector")

	if modelID == nil || *modelID == "" {
		model := Gemini31FlashLite
		modelID = &model
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is required to use Google Gemini models")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("error creating Google AI client: %w", err)
	}

	model := client.GenerativeModel(*modelID)
	// model.SetTemperature(0.9) //configurable?

	return &GoogleConnector{
		client:  client,
		model:   model,
		logger:  logger,
		modelID: *modelID,
	}, nil
}

func deduceGenaiType(typeStr string) genai.Type {
	switch typeStr {
	case "string":
		return genai.TypeString
	case "integer":
		return genai.TypeInteger
	case "number":
		return genai.TypeNumber
	case "boolean":
		return genai.TypeBoolean
	case "array":
		return genai.TypeArray
	case "object":
		return genai.TypeObject
	default:
		return genai.TypeString // Default to string if unknown
	}
}

func convertInputSchemaToGenaiSchema(inputSchema map[string]any) (*genai.Schema, error) {
	logger := *utils.GetLogger()
	if inputSchema == nil {
		return nil, fmt.Errorf("input schema is nil")
	}
	// Check if the input schema is of type object
	if inputSchema["type"] != "object" {
		return nil, fmt.Errorf("input schema is not of type object")
	}
	// Create a new GenAI schema
	schema := &genai.Schema{
		Type:       genai.TypeObject,
		Properties: make(map[string]*genai.Schema),
		Required:   []string{},
	}

	properties, ok := inputSchema["properties"].(map[string]any)
	if !ok {
		return schema, fmt.Errorf("properties are not defined in the input schema")
	}

	for propName, propDef := range properties {
		var propDefMap map[string]any
		isMap := false

		// Try asserting to map[string]any
		if m, ok := propDef.(map[string]any); ok {
			propDefMap = m
			isMap = true
		} else if mStr, okStr := propDef.(map[string]string); okStr {
			// If it's map[string]string, convert it to map[string]any
			tempMap := make(map[string]any, len(mStr))
			for key, val := range mStr {
				tempMap[key] = val
			}
			propDefMap = tempMap
			isMap = true
		}

		// If it wasn't either expected map type, skip
		if !isMap {
			logger.Warn(fmt.Sprintf("property '%s' has unexpected type %T. skipping", propName, propDef))
			continue
		}

		desc, ok := propDefMap["description"].(string)
		if !ok {
			// desc = ""
			// logger.Sugar().Warnf("property '%s' description not found. skipping", propName)
			logger.Warn(fmt.Sprintf("property '%s' description not found. skipping", propName))
			continue
		}
		genaiSchema := &genai.Schema{
			Description: desc,
		}

		if typeStr, typeOk := propDefMap["type"].(string); typeOk {
			genaiSchema.Type = deduceGenaiType(typeStr)
		}

		if genaiSchema.Type == genai.TypeArray {
			var itemsMap map[string]any
			switch items := propDefMap["items"].(type) {
			case map[string]any:
				itemsMap = items
			case map[string]string:
				itemsMap = make(map[string]any, len(items))
				for key, val := range items {
					itemsMap[key] = val
				}
			}
			if itemsMap != nil {
				if itemType, itemOk := itemsMap["type"].(string); itemOk {
					desc, ok := itemsMap["description"].(string)
					if !ok {
						logger.Warn(fmt.Sprintf("item property '%s' description not found. skipping", propName))
						continue
					}
					genaiSchema.Items = &genai.Schema{
						Type:        deduceGenaiType(itemType),
						Description: desc,
					}
				}
			}
		} else if genaiSchema.Type == genai.TypeObject {
			err := fmt.Errorf("nested objects are not supported in Google AI schema conversion")
			return nil, err
		}

		schema.Properties[propName] = genaiSchema

		for _, requiredProp := range requiredFields(inputSchema) {
			if requiredProp == propName {
				schema.Required = append(schema.Required, propName)
				break
			}
		}
	}

	return schema, nil
}

func convertToolsToGoogle(execTools map[string]tools.Tool) ([]*genai.Tool, error) {
	// Define the input schema as a map
	var toolSpecs []*genai.Tool
	for _, tool := range execTools {
		// Create the tool specification
		toolParams, err := convertInputSchemaToGenaiSchema(tool.InputSchema())
		if err != nil {
			err = fmt.Errorf("error converting tool input schema: %w", err)
			return nil, err
		}
		toolSpec := &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  toolParams,
			}},
		}
		toolSpecs = append(toolSpecs, toolSpec)
	}
	return toolSpecs, nil
}

func (gc *GoogleConnector) Query(ctx context.Context, qParams *QueryParams) (string, error) {
	gc.model.SystemInstruction = genai.NewUserContent(genai.Text(*qParams.SysPrompt))
	if len(qParams.Messages) > 0 {
		session := gc.model.StartChat()
		for _, msg := range qParams.Messages {
			var content genai.Part
			content = genai.Text(msg.Content)
			if msg.Role == "assistant" {
				session.History = append(session.History, &genai.Content{Role: "model", Parts: []genai.Part{content}})
			} else {
				session.History = append(session.History, &genai.Content{Role: "user", Parts: []genai.Part{content}})
			}
		}
		resp, err := session.SendMessage(ctx, genai.Text(*qParams.UserPrompt))
		if err != nil {
			return "", fmt.Errorf("error sending message to Google AI: %w", err)
		}
		if text, ok := firstCandidateText(resp); ok {
			return text, nil
		}
		return "", ErrNoProviderResponse
	}

	resp, err := gc.model.GenerateContent(ctx, genai.Text(*qParams.UserPrompt))
	if err != nil {
		return "", fmt.Errorf("error sending message to Google AI: %w", err)
	}

	if text, ok := firstCandidateText(resp); ok {
		return text, nil
	}

	return "", ErrNoProviderResponse
}

// firstCandidateText returns the first text part of the response's first usable
// candidate. It is nil-safe at every level (nil response, nil candidate
// pointer, withheld/nil content, no parts): genai's Candidates is a []*Candidate
// and Candidate.Content is a *Content that Gemini leaves nil for filtered or
// safety-blocked responses, so each must be checked before dereferencing.
func firstCandidateText(resp *genai.GenerateContentResponse) (string, bool) {
	if resp == nil || len(resp.Candidates) == 0 {
		return "", false
	}
	candidate := resp.Candidates[0]
	if candidate == nil || candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", false
	}
	return fmt.Sprint(candidate.Content.Parts[0]), true
}

func (gc *GoogleConnector) QueryWithTool(ctx context.Context, qParams *QueryParams, execTools map[string]tools.Tool) (LlmResponseWithTools, error) {
	logger := *utils.GetLogger()
	logger.Sugar().Debugw("Query with tool", "model", gc.modelID)
	response := LlmResponseWithTools{}

	gc.model.SystemInstruction = genai.NewUserContent(genai.Text(*qParams.SysPrompt))

	geminiTools, err := convertToolsToGoogle(execTools)
	if err != nil {
		gc.logger.Sugar().Errorw("error converting tools to Google AI", "error", err)
		return response, err
	}
	gc.model.Tools = geminiTools
	session := gc.model.StartChat()

	logger.Sugar().Debugw("Sending message to Google AI", "userPrompt", *qParams.UserPrompt)
	resp, err := session.SendMessage(ctx, genai.Text(*qParams.UserPrompt))
	if err != nil {
		return response, fmt.Errorf("error sending message to Google AI: %w", err)
	}
	logger.Sugar().Debugw("Received response from Google AI", "response", resp)
	if resp == nil || len(resp.Candidates) == 0 {
		return response, ErrNoProviderResponse
	}

	sawContent := false
	for _, candidate := range resp.Candidates {
		if candidate == nil || candidate.Content == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			sawContent = true
			funcall, ok := part.(genai.FunctionCall)

			if ok {
				response.ToolUse = true
				response.ToolName = funcall.Name
				response.ToolInput = funcall.Args
			} else {
				response.Response += fmt.Sprint(part) + "\n"
			}
		}
	}

	// A response whose candidates were all withheld (nil content) or empty is
	// not a valid answer; surface it as an error instead of a silent empty
	// result that looks like a successful blank reply.
	if !sawContent {
		return response, ErrNoProviderResponse
	}

	return response, nil
}

func (gc *GoogleConnector) SupportsNativeToolCalling() bool {
	return true
}
