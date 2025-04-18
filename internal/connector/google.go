package connector

import (
	"context"
	"fmt"
	"os"

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
		client:  client,
		model:   model,
		logger:  logger,
		modelID: *modelID,
	}

	return connector
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

// Google does not support tools
func (gc *GoogleConnector) QueryWithTool(ctx context.Context, params *QueryParams) (string, error) {
	return "", fmt.Errorf("Google connector does not support tools")
}
