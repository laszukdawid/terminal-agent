package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/history"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewQuestionCommand(config config.Config) *cobra.Command {
	var provider *string
	var modelID *string
	var promptFlag *string
	var contextFiles []string
	var terminalContextCount int
	var terminalContext1 bool
	var terminalContext2 bool
	var terminalContext3 bool
	var terminalContext4 bool
	var terminalContext5 bool

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
			service := app.NewService()

			// Check if this is streaming response
			streamFlag, _ := flags.GetBool("stream")
			plainFlag, _ := flags.GetBool("plain")
			memoryFlag, _ := flags.GetBool("memory")

			// Concatenate all remaining args to form the query
			userQuestion := strings.Join(args, " ")

			terminalContextCount, err := resolveTerminalContextCount(flags)
			if err != nil {
				return err
			}

			result, err := service.Ask(ctx, app.AskRequest{
				Message:              userQuestion,
				Provider:             *provider,
				Model:                *modelID,
				PromptOverride:       *promptFlag,
				UseMemory:            config.GetMemory() || memoryFlag,
				MemoryPath:           getMemoryPath(),
				WorkingDir:           config.GetWorkingDir(),
				ContextFiles:         contextFiles,
				TerminalContextCount: terminalContextCount,
				Stream:               streamFlag,
				Config:               config,
			})
			if err != nil {
				handleError(err)
				return nil
			}

			response := result.Response

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
				hClient.Log("ask", result.Question, response)
			}

			return nil
		},
	}

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

	// 'use-terminal-context' flag to include the latest terminal commands and output
	cmd.Flags().IntVar(&terminalContextCount, "use-terminal-context", 0, "Include latest N terminal commands and output as context (1-5, requires bash-reader plugin)")

	cmd.Flags().BoolVarP(&terminalContext1, "terminal-context-1", "1", false, "Shortcut for --use-terminal-context 1")
	cmd.Flags().BoolVarP(&terminalContext2, "terminal-context-2", "2", false, "Shortcut for --use-terminal-context 2")
	cmd.Flags().BoolVarP(&terminalContext3, "terminal-context-3", "3", false, "Shortcut for --use-terminal-context 3")
	cmd.Flags().BoolVarP(&terminalContext4, "terminal-context-4", "4", false, "Shortcut for --use-terminal-context 4")
	cmd.Flags().BoolVarP(&terminalContext5, "terminal-context-5", "5", false, "Shortcut for --use-terminal-context 5")

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
	return app.BuildContextFromFiles(files)
}

// BuildAskPrompt builds the system prompt for the ask command, optionally including memory
func BuildAskPrompt(basePrompt string, includeMemory bool, memoryPath string) (string, error) {
	return app.BuildAskPrompt(basePrompt, includeMemory, memoryPath)
}

func resolveTerminalContextCount(flags *pflag.FlagSet) (int, error) {
	if flags == nil {
		return 0, nil
	}

	useTerminalContext, err := flags.GetInt("use-terminal-context")
	if err != nil {
		return 0, fmt.Errorf("failed to parse --use-terminal-context: %w", err)
	}

	shortcutCounts := map[string]int{
		"terminal-context-1": 1,
		"terminal-context-2": 2,
		"terminal-context-3": 3,
		"terminal-context-4": 4,
		"terminal-context-5": 5,
	}

	shortcutSelected := 0
	for name, count := range shortcutCounts {
		enabled, err := flags.GetBool(name)
		if err != nil {
			return 0, fmt.Errorf("failed to parse -%d: %w", count, err)
		}

		if !enabled {
			continue
		}

		if shortcutSelected != 0 {
			return 0, fmt.Errorf("use only one terminal context shortcut flag (-1 to -5)")
		}

		shortcutSelected = count
	}

	if flags.Changed("use-terminal-context") {
		if useTerminalContext < 1 || useTerminalContext > 5 {
			return 0, fmt.Errorf("--use-terminal-context must be between 1 and 5")
		}

		if shortcutSelected != 0 {
			return 0, fmt.Errorf("cannot combine --use-terminal-context with terminal context shortcut flags (-1 to -5)")
		}

		return useTerminalContext, nil
	}

	if shortcutSelected != 0 {
		return shortcutSelected, nil
	}

	return 0, nil
}
