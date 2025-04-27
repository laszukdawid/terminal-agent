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
	Gemini20FlashLite = "gemini-2.0-flash-lite"
)

type GoogleConnector struct {
	client  *genai.Client
	model   *genai.GenerativeModel
	logger  zap.Logger
	modelID string
}

func NewGoogleConnector(modelID *string) *GoogleConnector {
	logger := *utils.GetLogger()
	logger.Debug("NewGoogleConnector")

	if modelID == nil || *modelID == "" {
		model := Gemini20FlashLite
		modelID = &model
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		logger.Error("GEMINI_API_KEY is required to use Google Gemini models")
		return nil
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		logger.Fatal(fmt.Sprintf("error creating Google AI client: %v", err))
		return nil
	}

	model := client.GenerativeModel(*modelID)
	// model.SetTemperature(0.9) //configurable?

	connector := &GoogleConnector{
		client:  client,
		model:   model,
		logger:  logger,
		modelID: *modelID,
	}

	return connector
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
		propDefMap, ok := propDef.(map[string]any)
		if !ok {
			continue
		}

		desc, ok := propDefMap["description"].(string)
		if !ok {
			// desc = ""
			utils.Logger.Sugar().Warnf("property '%s' description not found. skipping", propName)
			continue
		}
		genaiSchema := &genai.Schema{
			Description: desc,
		}

		if typeStr, typeOk := propDefMap["type"].(string); typeOk {
			genaiSchema.Type = deduceGenaiType(typeStr)
		}

		if genaiSchema.Type == genai.TypeArray {
			if items, ok := propDefMap["items"].(map[string]any); ok {
				if itemType, itemOk := items["type"].(string); itemOk {
					desc, ok := items["description"].(string)
					if !ok {
						// desc = ""
						utils.Logger.Sugar().Warnf("item property '%s' description not found. skipping", propName)
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

		if requiredSlice, ok := inputSchema["required"].([]string); ok {
			for _, requiredProp := range requiredSlice {
				if requiredProp == propName {
					schema.Required = append(schema.Required, propName)
					break
				}
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
	// cs := gc.model.StartChat()
	gc.model.SystemInstruction = genai.NewUserContent(genai.Text(*qParams.SysPrompt))
	resp, err := gc.model.GenerateContent(ctx, genai.Text(*qParams.UserPrompt))
	// resp, err := cs.SendMessage(ctx, genai.Text())
	if err != nil {
		return "", fmt.Errorf("error sending message to Google AI: %w", err)
	}

	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		return fmt.Sprint(resp.Candidates[0].Content.Parts[0]), nil
	}

	return "", fmt.Errorf("no response from Google AI")
}

func (gc *GoogleConnector) QueryWithTool(ctx context.Context, qParams *QueryParams, execTools map[string]tools.Tool) (string, error) {
	logger := *utils.GetLogger()
	logger.Sugar().Debugw("Query with tool", "model", gc.modelID)

	gc.model.SystemInstruction = genai.NewUserContent(genai.Text(*qParams.SysPrompt))

	geminiTools, err := convertToolsToGoogle(execTools)
	if err != nil {
		gc.logger.Sugar().Errorw("error converting tools to Google AI", "error", err)
		return "", err
	}
	gc.model.Tools = geminiTools
	session := gc.model.StartChat()

	logger.Sugar().Debugw("Sending message to Google AI", "userPrompt", *qParams.UserPrompt)
	resp, err := session.SendMessage(ctx, genai.Text(*qParams.UserPrompt))
	if err != nil {
		return "", fmt.Errorf("error sending message to Google AI: %w", err)
	}
	logger.Sugar().Debugw("Received response from Google AI", "response", resp)
	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("no response from Google AI")
	}

	// Check if the response contains a function call
	part := resp.Candidates[0].Content.Parts[0]
	funcall, ok := part.(genai.FunctionCall)
	if !ok {
		// If no function call, return the text response
		return fmt.Sprint(part), nil
	}

	// Check if the function call is valid
	execTool, exist := execTools[funcall.Name]
	if !exist {
		return "", fmt.Errorf("tool %s not found", funcall.Name)
	}

	// Execute the tool with the provided arguments
	logger.Sugar().Debugw("Executing tool", "toolName", funcall.Name, "arguments", funcall.Args)
	args := funcall.Args
	result, err := execTool.RunSchema(args)
	if err != nil {
		return "", fmt.Errorf("error executing tool %s: %w", funcall.Name, err)
	}

	return result, nil

}
