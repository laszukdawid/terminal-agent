package ask

import (
	"fmt"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/spf13/cobra"
)

type queryLog struct {
	Method    string `json:"method"`
	Timestamp string `json:"timestamp"`
	Query     string `json:"query"`
	Answer    string `json:"answer"`
}

func NewQuestionCommand() *cobra.Command {
	var userQuestion *string
	var provider *string
	var modelID *string

	cmd := &cobra.Command{
		Use: "ask",
		// SilenceErrors: true,
		SilenceUsage: true,
		Long:         "Sends a question to the underlying LLM model",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			connector := connector.NewConnector(*provider, *modelID)
			agent := agent.NewAgent(*connector)
			flags := cmd.Flags()

			response, err := agent.Question(ctx, *userQuestion)
			if err != nil {
				return fmt.Errorf("failed to ask question: %w", err)
			}

			if printFlag, err := flags.GetBool("print"); printFlag && err == nil {
				fmt.Println(response)
			}

			if logFlag, err := flags.GetBool("log"); logFlag && err == nil {
				// Write the response to the jsonl file
				l := queryLog{
					Method:    "ask",
					Query:     *userQuestion,
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

	cmd.MarkFlagRequired("question")
	userQuestion = cmd.Flags().StringP("query", "q", "", "The question to ask the model")
	provider = cmd.Flags().StringP("provider", "p", connector.PerplexityProvider, "The provider to use for the question")
	modelID = cmd.Flags().StringP("model", "m", "", "The model ID to use for the question")

	// 'print' flag whether to print response to the stdout (default: true)
	cmd.Flags().BoolP("print", "x", true, "Print the response to the stdout")

	// 'log' flag whether to log the input and output to a file (default: false)
	cmd.Flags().BoolP("log", "l", false, "Log the input and output to a file")

	return cmd
}
