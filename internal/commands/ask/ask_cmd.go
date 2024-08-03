package ask

import (
	"fmt"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/spf13/cobra"
)

var gProvider string
var gModelID string

const ()

type queryLog struct {
	Timestamp string `json:"timestamp"`
	Question  string `json:"question"`
	Answer    string `json:"answer"`
}

func NewQuestionCommand() *cobra.Command {
	var questionText *string

	cmd := &cobra.Command{
		Use:  "ask",
		Long: "Sends a question to the underlying LLM model",
		RunE: func(cmd *cobra.Command, args []string) error {
			connector := connector.NewConnector(gProvider, gModelID)
			agent := agent.NewAgent(connector)
			flags := cmd.Flags()

			response, err := agent.Question(*questionText)
			if err != nil {
				return fmt.Errorf("failed to ask question: %w", err)
			}

			if printFlag, err := flags.GetBool("print"); printFlag && err == nil {
				fmt.Println(response)
			}

			if logFlag, err := flags.GetBool("log"); logFlag && err == nil {
				// Write the response to the jsonl file
				l := queryLog{
					Question:  *questionText,
					Answer:    response,
					Timestamp: time.Now().Format(time.RFC3339),
				}

				if err := utils.WriteToJSONLFile("query_log.jsonl", l); err != nil {
					return fmt.Errorf("failed to write to jsonl file: %w", err)
				}
			}

			return nil
		},
	}

	questionText = cmd.Flags().StringP("question", "q", "", "The question to ask the model")
	cmd.MarkFlagRequired("question")

	cmd.Flags().StringVarP(&gProvider, "provider", "p", connector.PerplexityProvider, "The provider to use for the question")
	cmd.Flags().StringVarP(&gModelID, "model", "m", "", "The model ID to use for the question")

	// 'print' flag whether to print response to the stdout (default: true)
	cmd.Flags().BoolP("print", "x", true, "Print the response to the stdout")

	// 'log' flag whether to log the input and output to a file (default: false)
	cmd.Flags().BoolP("log", "l", false, "Log the input and output to a file")

	return cmd
}
