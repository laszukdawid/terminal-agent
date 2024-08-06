package task

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
	Request   string `json:"request"`
	Answer    string `json:"answer"`
}

// Simple mock connector for the task command
type mockConnector struct {
	provider string
	modelID  string
}

func NewMockConnector(provider, modelID string) *mockConnector {
	return &mockConnector{
		provider: provider,
		modelID:  modelID,
	}
}

func (m *mockConnector) Query(userPrompt *string, sysPrompt *string) (string, error) {
	return "prompt: " + *userPrompt, nil
}

func NewTaskCommand() *cobra.Command {
	var userRequest *string
	var provider *string
	var modelID *string

	cmd := &cobra.Command{
		Use:  "task",
		Long: "Execute a task using the underlying LLM model",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			connector := *connector.NewConnector(*provider, *modelID)
			// connector := NewMockConnector(*provider, *modelID)
			agent := agent.NewAgent(connector)
			flags := cmd.Flags()

			response, err := agent.Task(ctx, *userRequest)
			if err != nil {
				return fmt.Errorf("failed to request a task: %w", err)
			}

			if printFlag, err := flags.GetBool("print"); printFlag && err == nil {
				fmt.Println(response)
			}

			if logFlag, err := flags.GetBool("log"); logFlag && err == nil {
				// Write the response to the jsonl file
				l := queryLog{
					Method:    "task",
					Request:   *userRequest,
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

	cmd.MarkFlagRequired("task")
	userRequest = cmd.Flags().StringP("query", "q", "", "The task to perform")
	provider = cmd.Flags().StringP("provider", "p", connector.PerplexityProvider, "The provider to use for the question")
	modelID = cmd.Flags().StringP("model", "m", "", "The model ID to use for the question")

	// 'print' flag whether to print response to the stdout (default: true)
	cmd.Flags().BoolP("print", "x", true, "Print the response to the stdout")

	// 'log' flag whether to log the input and output to a file (default: false)
	cmd.Flags().BoolP("log", "l", false, "Log the input and output to a file")

	return cmd
}
