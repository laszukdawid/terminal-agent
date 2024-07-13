package ask

import (
	"fmt"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/spf13/cobra"
)

func NewQuestionCommand() *cobra.Command {
	var questionText *string

	cmd := &cobra.Command{
		Use:  "ask",
		Long: "Sends a question to the underlying LLM model",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := question(*questionText); err != nil {
				return err
			}
			return nil
		},
	}

	questionText = cmd.Flags().StringP("question", "q", "", "The question to ask the model")
	cmd.MarkFlagRequired("question")

	return cmd

}

func question(s string) error {
	fmt.Println("Asking the model: ", s)
	connector.AskBedrock(s)
	return nil
}
