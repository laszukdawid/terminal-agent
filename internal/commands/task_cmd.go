package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/history"
	"github.com/spf13/cobra"
)

var newService = app.NewService

func NewTaskCommand(config config.Config) *cobra.Command {
	var provider *string
	var modelID *string
	var promptFlag *string
	var allowList *[]string

	cmd := &cobra.Command{
		Use:          "task",
		Short:        "Execute a task using the underlying LLM model",
		SilenceUsage: true,
		Long: `Execute a task using the underlying LLM model

		Any remaining argument that isn't captured by the flags will be concatenated to form the query.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := cmd.Flags()
			service := newService()
			inputReader := bufio.NewReader(cmd.InOrStdin())
			device, err := resolveDevice(flags, config)
			if err != nil {
				return err
			}
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

			// Resolve the task timeout: an explicit --timeout (including 0 =
			// unlimited) wins; otherwise fall back to the configured default.
			taskTimeout := config.GetTaskTimeout()
			if flags.Changed("timeout") {
				if flagTimeout, err := flags.GetDuration("timeout"); err == nil {
					taskTimeout = flagTimeout
				}
			}

			events, err := service.TaskEvents(ctx, app.TaskRequest{
				Message:        userRequest,
				Provider:       *provider,
				Model:          *modelID,
				PromptOverride: *promptFlag,
				WorkingDir:     taskWorkingDir,
				Allow:          allow,
				Device:         device,
				Timeout:        taskTimeout,
				Config:         config,
			})
			if err != nil {
				return fmt.Errorf("failed to request a task: %w", err)
			}

			printFlag, printFlagErr := flags.GetBool("print")
			if printFlagErr != nil {
				printFlag = true
			}

			result := app.TaskResult{Request: userRequest}
			printedStreamedToolOutput := false
			lastStreamChunkEndedWithNewline := true
			for event := range events {
				switch event.Type {
				case app.EventOutputDelta:
					if printFlag {
						printedStreamedToolOutput = true
						if event.Text != "" {
							lastStreamChunkEndedWithNewline = strings.HasSuffix(event.Text, "\n")
						}
						cmd.Print(event.Text)
					}
				case app.EventWarning:
					if event.Text != "" {
						cmd.PrintErrf("Warning: %s\n", event.Text)
					}
				case app.EventConfirmationNeeded:
					decision, promptErr := promptTaskConfirmation(cmd, inputReader, event.Confirmation)
					if promptErr != nil {
						return promptErr
					}
					if replyErr := event.Confirmation.Reply(decision); replyErr != nil {
						return replyErr
					}
				case app.EventClarificationNeeded:
					answer, promptErr := promptTaskClarification(cmd, inputReader, event.Clarification)
					if promptErr != nil {
						return promptErr
					}
					if replyErr := event.Clarification.Reply(answer); replyErr != nil {
						return replyErr
					}
				case app.EventCompleted:
					result.Response = event.FinalOutput
					if !printedStreamedToolOutput {
						result.RawOutput = event.RawOutput
					}
					result.RawOutputTool = event.RawOutputTool
					result.DirectRawOutput = event.DirectRawOutput
				case app.EventFailed:
					return fmt.Errorf("failed to request a task: %w", event.Err)
				}
			}

			response := result.Response

			if printedStreamedToolOutput && result.DirectRawOutput {
				response = ""
			}

			if printFlag && response != "" {
				if printedStreamedToolOutput && !lastStreamChunkEndedWithNewline {
					cmd.Print("\n")
				}
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

	// 'timeout' flag bounds the whole task run (Go duration, e.g. 15m). 0 means unlimited.
	// Defaults to unlimited unless task_timeout is set in config.
	cmd.Flags().Duration("timeout", 0, "Maximum task duration (e.g. 90s, 15m, 2h); 0 means no timeout")

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

func promptTaskConfirmation(cmd *cobra.Command, reader *bufio.Reader, confirmation *app.TaskConfirmationEvent) (app.TaskConfirmationResponse, error) {
	if confirmation == nil {
		return app.TaskConfirmationResponse{}, fmt.Errorf("missing confirmation request")
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Execute the following action?\n > %s [y/N/yes!/no!]: ", confirmation.Action); err != nil {
		return app.TaskConfirmationResponse{}, err
	}

	response, err := reader.ReadString('\n')
	if err != nil {
		return app.TaskConfirmationResponse{}, err
	}

	switch strings.TrimSpace(strings.ToLower(response)) {
	case "y", "yes":
		return app.TaskConfirmationResponse{Allowed: true}, nil
	case "y!", "yes!":
		return app.TaskConfirmationResponse{Allowed: true, Remember: true}, nil
	case "n!", "no!":
		return app.TaskConfirmationResponse{Remember: true}, nil
	default:
		return app.TaskConfirmationResponse{}, nil
	}
}

func promptTaskClarification(cmd *cobra.Command, reader *bufio.Reader, clarification *app.TaskClarificationEvent) (string, error) {
	if clarification == nil {
		return "", fmt.Errorf("missing clarification request")
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "\nNeed clarification: %s\n> ", clarification.Question); err != nil {
		return "", err
	}

	response, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimRight(response, "\r\n"), nil
}
