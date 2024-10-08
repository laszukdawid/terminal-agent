package ask

import (
	"fmt"
	"os"
	"strings"
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
	var provider *string
	var modelID *string

	cmd := &cobra.Command{
		Use:          "ask",
		SilenceUsage: true,
		Short:        "Sends a question to the underlying LLM model",
		Long: `Sends a question to the underlying LLM model

		Any remaining argument that isn't captured by the flags will be concatenated to form the question.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			connector := connector.NewConnector(*provider, *modelID)
			agent := agent.NewAgent(*connector)
			flags := cmd.Flags()

			// Concatenate all remaining args to form the query
			userQuestion := strings.Join(args, " ")

			response, err := agent.Question(ctx, userQuestion)
			if err != nil {
				handleError(err)
				return nil
			}

			if printFlag, err := flags.GetBool("print"); printFlag && err == nil {
				fmt.Println(response)
			}

			if logFlag, err := flags.GetBool("log"); logFlag && err == nil {
				// Write the response to the jsonl file
				l := queryLog{
					Method:    "ask",
					Query:     userQuestion,
					Answer:    response,
					Timestamp: time.Now().Format(time.RFC3339),
				}

				if err := utils.WriteToJSONLFile("query_log.jsonl", l); err != nil {
					return fmt.Errorf("failed to write to jsonl file: %w", err)
				}
			}

			return nil
		},
		Args: cobra.MinimumNArgs(1),
	}

	cmd.MarkFlagRequired("question")
	provider = cmd.Flags().StringP("provider", "p", "", "The provider to use for the question")
	modelID = cmd.Flags().StringP("model", "m", "", "The model ID to use for the question")

	if *provider == "" {
		*provider = os.Getenv("PROVIDER")
	}
	if *provider == "" {
		*provider = connector.PerplexityProvider
	}

	// 'print' flag whether to print response to the stdout (default: true)
	cmd.Flags().BoolP("print", "x", true, "Print the response to the stdout")

	// 'log' flag whether to log the input and output to a file (default: false)
	cmd.Flags().BoolP("log", "l", false, "Log the input and output to a file")

	return cmd
}

func handleError(err error) {
	if err == nil {
		panic("handleError called with nil error")
	}
	switch err {
	case agent.ErrEmptyQuery:
		fmt.Println("Query is empty")
	case connector.ErrPerplexityForbidden:
		fmt.Println("Couldn't authenticate with the Perplexity API. Make sure you have the correct API key in the environment variable PERPLEXITY_KEY.")
	case connector.ErrBedrockForbidden:
		fmt.Println("Couldn't authenticate with the Bedrock API. Make sure you have the correct AWS credentials set up.")
	}
	fmt.Println(err)
	os.Exit(1)
}
