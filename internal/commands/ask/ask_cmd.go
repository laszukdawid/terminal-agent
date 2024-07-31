package ask

import (
	"github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/spf13/cobra"
)

var gProvider string
var gModelID string

const ()

func NewQuestionCommand() *cobra.Command {
	var questionText *string

	cmd := &cobra.Command{
		Use:  "ask",
		Long: "Sends a question to the underlying LLM model",
		RunE: func(cmd *cobra.Command, args []string) error {
			connector := connector.NewConnector(gProvider, gModelID)
			agent := agent.NewAgent(connector)
			if err := agent.Question(*questionText); err != nil {
				return err
			}

			return nil
		},
	}

	questionText = cmd.Flags().StringP("question", "q", "", "The question to ask the model")
	cmd.MarkFlagRequired("question")

	cmd.Flags().StringVarP(&gProvider, "provider", "p", connector.PerplexityProvider, "The provider to use for the question")
	cmd.Flags().StringVarP(&gModelID, "model", "m", "", "The model ID to use for the question")

	return cmd
}
