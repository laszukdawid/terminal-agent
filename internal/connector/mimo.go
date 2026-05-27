package connector

import (
	"fmt"
	"os"

	"github.com/laszukdawid/terminal-agent/internal/auth"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const (
	MiMoProvider = "mimo"

	DefaultMiMoModel   = "mimo-v2.5-pro"
	DefaultMiMoBaseURL = "https://api.xiaomimimo.com/v1"
)

type MiMoConnector struct {
	*OpenAIConnector
}

func NewMiMoConnector(modelID *string) *MiMoConnector {
	logger := *utils.GetLogger()
	logger.Debug("NewMiMoConnector")

	if modelID == nil || *modelID == "" {
		model := DefaultMiMoModel
		modelID = &model
	}

	apiKey := os.Getenv("MIMO_API_KEY")
	clientOptions := []option.RequestOption{option.WithBaseURL(getMiMoBaseURL())}
	authErr := error(nil)
	if apiKey == "" {
		authErr = fmt.Errorf("MIMO_API_KEY is required to use Xiaomi MiMo models")
	} else {
		clientOptions = append(clientOptions, option.WithAPIKey(apiKey))
	}

	client := openai.NewClient(clientOptions...)

	return &MiMoConnector{
		OpenAIConnector: &OpenAIConnector{
			client:  &client,
			logger:  logger,
			modelID: *modelID,
			auth: auth.ResolvedAuth{
				Type:  auth.CredentialTypeAPIKey,
				Token: apiKey,
			},
			authErr: authErr,
		},
	}
}

func getMiMoBaseURL() string {
	if baseURL := os.Getenv("MIMO_BASE_URL"); baseURL != "" {
		return baseURL
	}
	return DefaultMiMoBaseURL
}
