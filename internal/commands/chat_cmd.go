package commands

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/spf13/cobra"
)

func getChatDBPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".local", "share", "terminal-agent", "chat.db")
}

func NewChatCommand(config config.Config) *cobra.Command {
	var provider *string
	var modelID *string
	var promptFlag *string
	var newSession bool
	var contextFiles []string

	cmd := &cobra.Command{
		Use:          "chat",
		SilenceUsage: true,
		Args:         cobra.MinimumNArgs(1),
		Short:        "Chat with the LLM while maintaining conversation history",
		Long: `Chat with the LLM while maintaining conversation history across invocations.

Examples:
  agent chat "how to check what commits were added?"
  agent chat "how to check just the last 2 commits"
  agent chat --new "start a fresh conversation"

The conversation history is persisted between calls. Use --new to start a fresh session.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			flags := cmd.Flags()
			service := app.NewService()
			execConfig := config
			device, err := resolveDevice(flags, execConfig)
			if err != nil {
				return err
			}

			streamFlag, _ := flags.GetBool("stream")
			plainFlag, _ := flags.GetBool("plain")
			memoryFlag, _ := flags.GetBool("memory")

			// Concatenate all remaining args to form the message
			userMessage := strings.Join(args, " ")

			result, err := service.Chat(ctx, app.ChatRequest{
				Message:        userMessage,
				Provider:       *provider,
				Model:          *modelID,
				PromptOverride: *promptFlag,
				UseMemory:      execConfig.GetMemory() || memoryFlag,
				MemoryPath:     getMemoryPath(),
				ContextFiles:   contextFiles,
				Stream:         streamFlag,
				Device:         device,
				NewSession:     newSession,
				ChatDBPath:     getChatDBPath(),
				Config:         execConfig,
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

			return nil
		},
	}

	provider = cmd.Flags().StringP("provider", "p", config.GetDefaultProvider(), "The provider to use")
	modelID = cmd.Flags().StringP("model", "m", config.GetDefaultModelId(), "The model ID to use")
	promptFlag = cmd.Flags().String("prompt", "", "Custom system prompt (overrides file-based and default prompts)")

	if *provider == "" {
		*provider = os.Getenv("PROVIDER")
	}
	if *provider == "" {
		*provider = connector.PerplexityProvider
	}

	cmd.Flags().BoolP("print", "x", true, "Print the response to the stdout")
	cmd.Flags().BoolP("stream", "s", false, "Stream the response to the stdout")
	cmd.Flags().BoolP("plain", "k", false, "Render the response as plain text")
	cmd.Flags().BoolP("memory", "M", false, "Include memory in the system prompt")
	cmd.Flags().BoolVarP(&newSession, "new", "n", false, "Start a new chat session")
	cmd.Flags().StringArrayVarP(&contextFiles, "context", "c", []string{}, "Include file content as context (can be used multiple times)")

	return cmd
}
