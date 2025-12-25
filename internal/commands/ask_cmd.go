package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/history"
	"github.com/laszukdawid/terminal-agent/internal/memory"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/spf13/cobra"
)

func NewQuestionCommand(config config.Config) *cobra.Command {
	var provider *string
	var modelID *string
	var promptFlag *string
	var contextFiles []string

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
			plainFlag, _ := flags.GetBool("plain")
			memoryFlag, _ := flags.GetBool("memory")

			// Resolve system prompts
			workingDir := config.GetWorkingDir()
			askPrompt, err := agent.ResolvePrompt(*promptFlag, "ask", workingDir)
			if err != nil {
				return fmt.Errorf("failed to resolve ask prompt: %w", err)
			}
			taskPrompt, err := agent.ResolvePrompt("", "task", workingDir)
			if err != nil {
				return fmt.Errorf("failed to resolve task prompt: %w", err)
			}

			// Include memory in prompt if enabled via config or flag
			includeMemory := config.GetMemory() || memoryFlag
			askPrompt, err = BuildAskPrompt(askPrompt, includeMemory, getMemoryPath())
			if err != nil {
				return err
			}

			connector := connector.NewConnector(*provider, *modelID)
			toolProvider := tools.NewToolProvider(config)
			agent := agent.NewAgent(*connector, toolProvider, config, askPrompt, taskPrompt)

			// Concatenate all remaining args to form the query
			userQuestion := strings.Join(args, " ")

			// Prepend context from files if provided
			if len(contextFiles) > 0 {
				contextContent, err := buildContextFromFiles(contextFiles)
				if err != nil {
					return fmt.Errorf("failed to read context files: %w", err)
				}
				userQuestion = contextContent + "\n\n" + userQuestion
			}

			response, err := agent.Question(ctx, userQuestion, streamFlag)
			if err != nil {
				handleError(err)
				return nil
			}

			// Print only if not streaming, and explicitly asked for printing
			if !streamFlag {
				if printFlag, err := flags.GetBool("print"); printFlag && err == nil {
					if !plainFlag {
						response = handleMarkdown(response)
					}
					cmd.Println(response)
				}
			}

			if logFlag, err := flags.GetBool("log"); logFlag && err == nil {
				hClient := history.NewHistory(getLogPath())
				hClient.Log("ask", userQuestion, response)
			}

			return nil
		},
	}

	cmd.MarkFlagRequired("question")
	provider = cmd.Flags().StringP("provider", "p", config.GetDefaultProvider(), "The provider to use for the question")
	modelID = cmd.Flags().StringP("model", "m", config.GetDefaultModelId(), "The model ID to use for the question")
	promptFlag = cmd.Flags().String("prompt", "", "Custom system prompt (overrides file-based and default prompts)")

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

	// 'plain' flag whether to render the response as plain text (default: false)
	cmd.Flags().BoolP("plain", "k", false, "Render the response as plain text")

	// 'memory' flag whether to include memory in the system prompt (default: false)
	cmd.Flags().BoolP("memory", "M", false, "Include memory in the system prompt")

	// 'context' flag to include file contents as context (can be used multiple times)
	cmd.Flags().StringArrayVarP(&contextFiles, "context", "c", []string{}, "Include file content as context (can be used multiple times)")

	// Add help subcommand that shows the same help as the parent command
	helpCmd := &cobra.Command{
		Use:   "help",
		Short: "Help about the ask command",
		Run: func(cmd *cobra.Command, args []string) {
			// Display the parent command's help
			cmd.Parent().Help()
		},
	}
	cmd.AddCommand(helpCmd)

	return cmd
}

func handleMarkdown(inString string) string {
	var response string
	if inString == "" {
		return ""
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)
	if err != nil {
		fmt.Println("Error rendering markdown:", err)
		return ""
	}
	// Render the response as markdown into the response variable
	response, err = r.Render(inString)
	if err != nil {
		fmt.Println("Error rendering markdown:", err)
		return ""
	}
	return response
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

// buildContextFromFiles reads multiple files and wraps their contents in <context> tags
func buildContextFromFiles(files []string) (string, error) {
	var contextParts []string
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("failed to read file %s: %w", file, err)
		}
		contextParts = append(contextParts, fmt.Sprintf("<context>\n%s\n</context>", strings.TrimSpace(string(content))))
	}
	return strings.Join(contextParts, "\n\n"), nil
}

// BuildAskPrompt builds the system prompt for the ask command, optionally including memory
func BuildAskPrompt(basePrompt string, includeMemory bool, memoryPath string) (string, error) {
	if !includeMemory {
		return basePrompt, nil
	}

	mClient := memory.NewMemory(memoryPath)
	memoryPrompt, err := mClient.FormatAsPrompt()
	if err != nil {
		return "", fmt.Errorf("failed to format memory: %w", err)
	}

	if memoryPrompt == "" {
		return basePrompt, nil
	}

	return memoryPrompt + "\n\n" + basePrompt, nil
}
