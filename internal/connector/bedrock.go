package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

type BedrockModelID string

const (
	BedrockProvider = "bedrock"

	ClaudeHaiku BedrockModelID = "anthropic.claude-3-haiku-20240307-v1:0"
)

// const modelId = "anthropic.claude-v2"

type BedrockConnector struct {
	client  *bedrockruntime.Client
	modelID BedrockModelID
}

func NewBedrockConnector(modelID *BedrockModelID) *BedrockConnector {
	// sdkConfig, err := cloud.NewAwsConfigWithSSO(context.Background(), "dev")
	// sdkConfig, err := cloud.NewAwsConfig(context.Background())
	sdkConfig, err := config.LoadDefaultConfig(context.Background(), config.WithRegion("us-east-1"))
	if err != nil {
		fmt.Println("Couldn't load default configuration. Have you set up your AWS account?")
		fmt.Println(err)
		return nil
	}

	if modelID == nil || *modelID == "" {
		model := ClaudeHaiku
		modelID = &model
	}

	client := bedrockruntime.NewFromConfig(sdkConfig)

	return &BedrockConnector{
		client:  client,
		modelID: *modelID,
	}
}

func (bc *BedrockConnector) Query(userPrompt *string, systemPrompt *string) (string, error) {

	request := ClaudeRequest{
		System: *systemPrompt,
		Messages: []Message{
			{Role: "user", Content: *userPrompt}},
		AnthropicVersion:  "bedrock-2023-05-31",
		MaxTokensToSample: 200,
	}

	body, err := json.Marshal(request)
	if err != nil {
		log.Panicln("Couldn't marshal the request: ", err)
	}

	result, err := bc.client.InvokeModel(context.Background(), &bedrockruntime.InvokeModelInput{
		ModelId:     (*string)(&bc.modelID),
		ContentType: aws.String("application/json"),
		Body:        body,
	})

	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}

	var response ClaudeResponse
	err = json.Unmarshal(result.Body, &response)

	if err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %v", err)
	}
	return response.Content[0].Text, nil
}
