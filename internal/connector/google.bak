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

	execTools map[string]tools.Tool
	toolSpecs []*genai.Tool
}

func NewGoogleConnector(modelID *string, execTools map[string]tools.Tool) *GoogleConnector {
	logger := *utils.GetLogger()
	logger.Debug("NewGoogleConnector")

	if modelID == nil || *modelID == "" {
		model := Gemini20FlashLite
		modelID = &model
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		logger.Fatal("GEMINI_API_KEY is required to use Google Gemini models")
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
		client:    client,
		model:     model,
		logger:    logger,
		modelID:   *modelID,
		execTools: execTools,
		toolSpecs: convertToolsToGoogle(execTools),
	}

	return connector
}

func convertToGenaiSchema(inputSchema map[string]any) *genai.Schema {
	schema := &genai.Schema{
		Type:       genai.TypeObject,
		Properties: make(map[string]*genai.Schema),
		Required:   []string{},
	}

	properties, ok := inputSchema["properties"].(map[string]any)
	if !ok {
		return schema
	}

	for propName, propDef := range properties {
		propDefMap, ok := propDef.(map[string]any)
		if !ok {
			continue
		}

		genaiSchema := &genai.Schema{
			Description: propDefMap["description"].(string),
		}

		typeStr, typeOk := propDefMap["type"].(string)
		if typeOk {
			switch typeStr {
			case "string":
				genaiSchema.Type = genai.TypeString
			case "integer":
				genaiSchema.Type = genai.TypeInteger
			case "number":
				genaiSchema.Type = genai.TypeNumber
			case "boolean":
				genaiSchema.Type = genai.TypeBoolean
			case "array":
				genaiSchema.Type = genai.TypeArray
			case "object":
				genaiSchema.Type = genai.TypeObject
			default:
				genaiSchema.Type = genai.TypeString // Default to string if unknown
			}
		}

		schema.Properties[propName] = genaiSchema

		// Check if the property is required
		if requiredSlice, ok := inputSchema["required"].([]string); ok {
			for _, requiredProp := range requiredSlice {
				if requiredProp == propName {
					schema.Required = append(schema.Required, propName)
					break
				}
			}
		}
	}

	return schema
}

func convertToolsToGoogle(execTools map[string]tools.Tool) []*genai.Tool {
	// Define the input schema as a map
	var toolSpecs []*genai.Tool
	for _, tool := range execTools {
		// Convert the input schema to a map[string]*genai.Schema
		inputSchema := make(map[string]*genai.Schema)
		for key, value := range tool.InputSchema() {
			inputSchema[key] = &genai.Schema{
				Type:        genai.TypeString,
				Description: value.(map[string]string)["description"],
			}
		}

		// Create the tool specification
		toolSpec := &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  convertToGenaiSchema(tool.InputSchema()),
			}},
		}
		toolSpecs = append(toolSpecs, toolSpec)
	}
	return toolSpecs
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

func (gc *GoogleConnector) QueryWithTool(ctx context.Context, qParams *QueryParams) (string, error) {
	gc.logger.Sugar().Debugw("Query with tool", "model", gc.modelID)
	gc.model.Tools = gc.toolSpecs

	gc.model.SystemInstruction = genai.NewUserContent(genai.Text(*qParams.SysPrompt))
	session := gc.model.StartChat()

	resp, err := session.SendMessage(ctx, genai.Text(*qParams.UserPrompt))
	if err != nil {
		return "", fmt.Errorf("error sending message to Google AI: %w", err)
	}

	part := resp.Candidates[0].Content.Parts[0]
	funcall, ok := part.(genai.FunctionCall)
	if ok {
		// Call the tool
		execTool, exist := gc.execTools[funcall.Name]
		if !exist {
			return "", fmt.Errorf("tool %s not found", funcall.Name)
		}

		args := funcall.Args
		result, err := execTool.RunSchema(args)
		if err != nil {
			return "", fmt.Errorf("error executing tool %s: %w", funcall.Name, err)
		}

		return result, nil
	}

	return "", fmt.Errorf("no response from Google AI")
}
