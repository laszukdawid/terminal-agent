package connector

import (
	"os"

	"github.com/laszukdawid/terminal-agent/internal/auth"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const (
	VLLMProvider       = "vllm"
	DefaultVLLMModel   = "meta-llama/Llama-3.1-8B-Instruct"
	DefaultVLLMBaseURL = "http://localhost:8000/v1"
)

type VLLMConnector struct {
	*OpenAIConnector
}

func NewVLLMConnector(modelID *string) *VLLMConnector {
	logger := *utils.GetLogger()
	logger.Debug("NewVLLMConnector")

	if modelID == nil || *modelID == "" {
		model := DefaultVLLMModel
		modelID = &model
	}

	apiKey := os.Getenv("VLLM_API_KEY")
	clientOptions := []option.RequestOption{
		option.WithBaseURL(getVLLMBaseURL()),
	}
	resolvedAuth := auth.ResolvedAuth{}
	if apiKey != "" {
		clientOptions = append(clientOptions, option.WithAPIKey(apiKey))
		resolvedAuth = auth.ResolvedAuth{
			Type:  auth.CredentialTypeAPIKey,
			Token: apiKey,
		}
	} else {
		// openai-go automatically reads OPENAI_API_KEY from the environment.
		// vLLM should default to unauthenticated local access unless explicitly configured.
		clientOptions = append(clientOptions, option.WithHeaderDel("Authorization"))
	}

	client := openai.NewClient(clientOptions...)

	return &VLLMConnector{
		OpenAIConnector: &OpenAIConnector{
			client:  &client,
			logger:  logger,
			modelID: *modelID,
			auth:    resolvedAuth,
		},
	}
}

func getVLLMBaseURL() string {
	if baseURL := os.Getenv("VLLM_BASE_URL"); baseURL != "" {
		return baseURL
	}
	return DefaultVLLMBaseURL
}
