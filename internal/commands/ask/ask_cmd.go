package ask

import (
	"fmt"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/spf13/cobra"
)

var gProvider string
var gModelID string

const (
	bedrockProvider    = "bedrock"
	perplexityProvider = "perplexity"
)

func NewQuestionCommand() *cobra.Command {
	var questionText *string

	cmd := &cobra.Command{
		Use:  "ask",
		Long: "Sends a question to the underlying LLM model",
		RunE: func(cmd *cobra.Command, args []string) error {
			if gProvider != bedrockProvider && gProvider != perplexityProvider {
				return fmt.Errorf("invalid provider: %s", gProvider)
			}

			if err := question(gProvider, gModelID, *questionText); err != nil {
				return err
			}
			return nil
		},
	}

	questionText = cmd.Flags().StringP("question", "q", "", "The question to ask the model")
	cmd.MarkFlagRequired("question")

	cmd.Flags().StringVarP(&gProvider, "provider", "p", perplexityProvider, "The provider to use for the question")
	cmd.Flags().StringVarP(&gModelID, "model", "m", "", "The model ID to use for the question")

	return cmd

}

func question(provider string, modelID string, s string) error {
	fmt.Println("Asking the model: ", s)
	var response string
	if provider == bedrockProvider {
		response = connector.AskBedrock(s)
	} else if provider == perplexityProvider {
		response = connector.AskPerplexity(s, (*connector.PerplexityModelId)(&modelID))
	}

	if response != "" {
		fmt.Println("Response: ", response)
	} else {
		fmt.Println("No response received")
	}
	return nil
}
