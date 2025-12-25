package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/chat"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
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

			// Initialize session store
			sessionStore, err := chat.NewSessionStore(getChatDBPath())
			if err != nil {
				return fmt.Errorf("failed to initialize chat session: %w", err)
			}
			defer sessionStore.Close()

			// Create new session if requested
			if newSession {
				_, err = sessionStore.NewSession()
				if err != nil {
					return fmt.Errorf("failed to create new session: %w", err)
				}
			}

			// Get or create session
			_, err = sessionStore.GetOrCreateSession()
			if err != nil {
				return fmt.Errorf("failed to get session: %w", err)
			}

			// Load conversation history
			chatHistory, err := sessionStore.GetMessages()
			if err != nil {
				return fmt.Errorf("failed to load chat history: %w", err)
			}

			// Convert chat.Message to connector.Message
			var connectorMessages []connector.Message
			for _, msg := range chatHistory {
				connectorMessages = append(connectorMessages, connector.Message{
					Role:    msg.Role,
					Content: msg.Content,
				})
			}

			// Create connector and agent
			conn := connector.NewConnector(*provider, *modelID)
			toolProvider := tools.NewToolProvider(config)
			agentInstance := agent.NewAgent(*conn, toolProvider, config, askPrompt, taskPrompt)

			// Concatenate all remaining args to form the message
			userMessage := strings.Join(args, " ")

			// Prepend context from files if provided
			if len(contextFiles) > 0 {
				contextContent, err := buildContextFromFiles(contextFiles)
				if err != nil {
					return fmt.Errorf("failed to read context files: %w", err)
				}
				userMessage = contextContent + "\n\n" + userMessage
			}

			// Save user message to session
			if err := sessionStore.AddMessage("user", userMessage); err != nil {
				return fmt.Errorf("failed to save user message: %w", err)
			}

			// Send message with history
			response, err := agentInstance.Chat(ctx, userMessage, connectorMessages, streamFlag)
			if err != nil {
				handleError(err)
				return nil
			}

			// Save assistant response to session
			if err := sessionStore.AddMessage("assistant", response); err != nil {
				return fmt.Errorf("failed to save assistant response: %w", err)
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
