package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

const defaultBedrockModelId = "anthropic.claude-3-haiku-20240307-v1:0"

// const modelId = "anthropic.claude-v2"

type LlmClient struct {
	BedrockRuntimeClient *bedrockruntime.Client
}

// Invokes Anthropic Claude on Amazon Bedrock to run an inference using the input
// provided in the request body.
func (wrapper LlmClient) InvokeClaude(prompt string) (string, error) {

	// Anthropic Claude requires enclosing the prompt as follows:
	enclosedPrompt := "Human: " + prompt + "\n\nAssistant:"

	body, err := json.Marshal(ClaudeRequest{
		Messages:          []Message{{Role: "user", Content: enclosedPrompt}},
		MaxTokensToSample: 200,
		Temperature:       0.1,
		StopSequences:     []string{"\n\nHuman:"},
	})

	if err != nil {
		log.Fatal("failed to marshal", err)
	}

	output, err := wrapper.BedrockRuntimeClient.InvokeModel(context.TODO(), &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(defaultBedrockModelId),
		ContentType: aws.String("application/json"),
		Body:        body,
	})

	if err != nil {
		fmt.Printf("Error: Couldn't invoke Anthropic Claude. Here's why: %v\n", err)
	}

	var response ClaudeResponse
	if err := json.Unmarshal(output.Body, &response); err != nil {
		log.Fatal("failed to unmarshal", err)
	}

	// return response.Content.Responses[0].Text, nil
	return "hello", nil
}

func AskBedrock(question string) string {

	// sdkConfig, err := cloud.NewAwsConfigWithSSO(context.Background(), "dev")
	// sdkConfig, err := cloud.NewAwsConfig(context.Background())
	sdkConfig, err := config.LoadDefaultConfig(context.Background(), config.WithRegion("us-east-1"))
	if err != nil {
		fmt.Println("Couldn't load default configuration. Have you set up your AWS account?")
		fmt.Println(err)
		return ""
	}

	client := bedrockruntime.NewFromConfig(sdkConfig)

	// Anthropic Claude requires you to enclose the prompt as follows:
	prefix := "Human: "
	postfix := "\n\nAssistant:"
	wrappedPrompt := prefix + question + postfix

	request := ClaudeRequest{
		Messages:          []Message{{Role: "user", Content: wrappedPrompt}},
		AntrhopicVersion:  "bedrock-2023-05-31",
		MaxTokensToSample: 200,
	}

	body, err := json.Marshal(request)
	if err != nil {
		log.Panicln("Couldn't marshal the request: ", err)
	}

	result, err := client.InvokeModel(context.Background(), &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(defaultBedrockModelId),
		ContentType: aws.String("application/json"),
		Body:        body,
	})

	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "no such host") {
			fmt.Printf("Error: The Bedrock service is not available in the selected region. Please double-check the service availability for your region at https://aws.amazon.com/about-aws/global-infrastructure/regional-product-services/.\n")
		} else if strings.Contains(errMsg, "Could not resolve the foundation model") {
			fmt.Printf("Error: Could not resolve the foundation model from model identifier: \"%v\". Please verify that the requested model exists and is accessible within the specified region.\n", defaultBedrockModelId)
		} else {
			fmt.Printf("Error: Couldn't invoke Anthropic Claude. Here's why: %v\n", err)
		}
		os.Exit(1)
	}

	var response ClaudeResponse
	err = json.Unmarshal(result.Body, &response)

	if err != nil {
		log.Fatal("failed to unmarshal", err)
	}
	fmt.Println("Response from Anthropic Claude")
	return response.Content[0].Text
}
