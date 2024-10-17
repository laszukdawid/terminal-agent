package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/spf13/cobra"
)

const (
	logFile = "query_log.jsonl"
)

var (
	logDir = filepath.Join(os.Getenv("HOME"), ".local", "share", "terminal-agent")
)

type askQueryLog struct {
	Method    string `json:"method"`
	Timestamp string `json:"timestamp"`
	Query     string `json:"query"`
	Answer    string `json:"answer"`
}

func NewQuestionCommand(config config.Config) *cobra.Command {
	var provider *string
	var modelID *string

	cmd := &cobra.Command{
		Use:          "ask",
		SilenceUsage: true,
		Args:         cobra.MinimumNArgs(1),
		Short:        "Sends a question to the underlying LLM model",
		Long: `Sends a question to the underlying LLM model

		Any remaining argument that isn't captured by the flags will be concatenated to form the question.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := cmd.Flags()

			// Check if this is streaming response
			streamFlag, _ := flags.GetBool("stream")

			connector := connector.NewConnector(*provider, *modelID)
			agent := agent.NewAgent(*connector)

			// Concatenate all remaining args to form the query
			userQuestion := strings.Join(args, " ")

			response, err := agent.Question(ctx, userQuestion, streamFlag)
			if err != nil {
				handleError(err)
				return nil
			}

			// Print only if not streaming, and explicitly asked for printing
			if !streamFlag {
				if printFlag, err := flags.GetBool("print"); printFlag && err == nil {
					fmt.Println(response)
				}
			}

			if logFlag, err := flags.GetBool("log"); logFlag && err == nil {
				// Write the response to the jsonl file
				l := askQueryLog{
					Method:    "ask",
					Query:     userQuestion,
					Answer:    response,
					Timestamp: time.Now().Format(time.RFC3339),
				}

				logPath := filepath.Join(logDir, logFile)
				if err := utils.WriteToJSONLFile(logPath, l); err != nil {
					return fmt.Errorf("failed to write to jsonl file: %w", err)
				}
			}

			return nil
		},
	}

	cmd.MarkFlagRequired("question")
	provider = cmd.Flags().StringP("provider", "p", config.GetDefaultProvider(), "The provider to use for the question")
	modelID = cmd.Flags().StringP("model", "m", config.GetDefaultModelId(), "The model ID to use for the question")

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

	// 'stream' flag whether to stream the response to the stdout (default: false)
	cmd.Flags().BoolP("stream", "s", false, "Stream the response to the stdout")

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
