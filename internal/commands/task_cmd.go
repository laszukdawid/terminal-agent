package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/history"
	"github.com/spf13/cobra"
)

func NewTaskCommand(config config.Config) *cobra.Command {
	var provider *string
	var modelID *string
	var promptFlag *string
	var allowList *[]string

	cmd := &cobra.Command{
		Use:   "task",
		Short: "Execute a task using the underlying LLM model",
		Long: `Execute a task using the underlying LLM model

		Any remaining argument that isn't captured by the flags will be concatenated to form the query.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := cmd.Flags()
			service := app.NewService()
			taskWorkingDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to resolve task working directory: %w", err)
			}

			// Concatenate all remaining args to form the query
			userRequest := strings.Join(args, " ")

			allow := []string{}
			if allowList != nil {
				allow = *allowList
			}

			result, err := service.Task(ctx, app.TaskRequest{
				Message:        userRequest,
				Provider:       *provider,
				Model:          *modelID,
				PromptOverride: *promptFlag,
				WorkingDir:     taskWorkingDir,
				Allow:          allow,
				Config:         config,
			})
			if err != nil {
				return fmt.Errorf("failed to request a task: %w", err)
			}

			response := result.Response

			if printFlag, err := flags.GetBool("print"); printFlag && err == nil {
				plain, _ := flags.GetBool("plain")
				cmd.Print(formatTaskOutput(app.TaskResult{
					Response:        response,
					RawOutput:       result.RawOutput,
					RawOutputTool:   result.RawOutputTool,
					DirectRawOutput: result.DirectRawOutput,
				}, plain))
			}

			if logFlag, err := flags.GetBool("log"); logFlag && err == nil {
				hClient := history.NewHistory(getLogPath())
				hClient.Log("task", result.Request, response)
			}

			return nil
		},
		Args: cobra.MinimumNArgs(1),
	}

	provider = cmd.Flags().StringP("provider", "p", config.GetDefaultProvider(), "The provider to use for the question")
	modelID = cmd.Flags().StringP("model", "m", config.GetDefaultModelId(), "The model ID to use for the question")
	promptFlag = cmd.Flags().String("prompt", "", "Custom system prompt (overrides file-based and default prompts)")
	allowList = cmd.Flags().StringArray("allow", []string{}, "Allow exact action without confirmation (repeatable)")

	// 'print' flag whether to print response to the stdout (default: true)
	cmd.Flags().BoolP("print", "x", true, "Print the response to the stdout")

	// 'plain' flag whether to render the response as plain text (default: false)
	cmd.Flags().BoolP("plain", "k", false, "Render the response as plain text")

	// 'log' flag whether to log the input and output to a file (default: false)
	cmd.Flags().BoolP("log", "l", false, "Log the input and output to a file")

	return cmd
}

func formatTaskOutput(result app.TaskResult, plain bool) string {
	response := result.Response
	if result.DirectRawOutput {
		if !strings.HasSuffix(response, "\n") {
			return response + "\n"
		}
		return response
	}
	if !plain {
		response = handleMarkdown(response)
	}

	if result.RawOutput == "" {
		return response
	}

	var output strings.Builder
	if response != "" {
		output.WriteString(response)
		if !strings.HasSuffix(response, "\n") {
			output.WriteByte('\n')
		}
		output.WriteByte('\n')
	}

	toolName := result.RawOutputTool
	if toolName == "" {
		toolName = "tool"
	}
	output.WriteString(fmt.Sprintf("Raw output from %s:\n", toolName))
	output.WriteString(result.RawOutput)
	if !strings.HasSuffix(result.RawOutput, "\n") {
		output.WriteByte('\n')
	}

	return output.String()
}
