package commands

import (
	"fmt"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/history"
	"github.com/spf13/cobra"
)

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

func NewTaskCommand(config config.Config) *cobra.Command {
	var provider *string
	var modelID *string

	cmd := &cobra.Command{
		Use:   "task",
		Short: "Execute a task using the underlying LLM model",
		Long: `Execute a task using the underlying LLM model

		Any remaining argument that isn't captured by the flags will be concatenated to form the query.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			connector := *connector.NewConnector(*provider, *modelID)
			// connector := NewMockConnector(*provider, *modelID)
			agent := agent.NewAgent(connector, config)
			flags := cmd.Flags()

			// Concatenate all remaining args to form the query
			userRequest := strings.Join(args, " ")

			response, err := agent.Task(ctx, userRequest)
			if err != nil {
				return fmt.Errorf("failed to request a task: %w", err)
			}

			if printFlag, err := flags.GetBool("print"); printFlag && err == nil {
				fmt.Println(response)
			}

			if logFlag, err := flags.GetBool("log"); logFlag && err == nil {
				hClient := history.NewHistory(getLogPath())
				hClient.Log("task", userRequest, response)
			}

			return nil
		},
		Args: cobra.MinimumNArgs(1),
	}

	cmd.MarkFlagRequired("task")
	provider = cmd.Flags().StringP("provider", "p", config.GetDefaultProvider(), "The provider to use for the question")
	modelID = cmd.Flags().StringP("model", "m", config.GetDefaultModelId(), "The model ID to use for the question")

	// 'print' flag whether to print response to the stdout (default: true)
	cmd.Flags().BoolP("print", "x", true, "Print the response to the stdout")

	// 'log' flag whether to log the input and output to a file (default: false)
	cmd.Flags().BoolP("log", "l", false, "Log the input and output to a file")

	return cmd
}
