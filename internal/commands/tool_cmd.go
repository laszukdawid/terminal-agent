package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/spf13/cobra"
)

func NewToolCommand(config config.Config) *cobra.Command {
	toolProvider := tools.NewToolProvider(config)
	allTools := toolProvider.GetAllTools()

	cmd := &cobra.Command{
		Use:          "tool",
		SilenceUsage: true,
		Short:        "Manage and execute tools",
		Long:         `Manage and execute tools.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// When no arguments or subcommands are provided, print help
			return cmd.Help()
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all available tools",
		Long:  `List all available tools, including built-in tools and tools from the MCP file if configured.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(allTools) == 0 {
				fmt.Println("No tools available")
				return nil
			}

			fmt.Println("Available tools:")
			fmt.Println("----------------")

			for name, tool := range allTools {
				fmt.Printf("- %s: %s\n", name, tool.Description())
			}

			return nil
		},
	}

	// Add help subcommand
	helpCmd := &cobra.Command{
		Use:   "help [toolName]",
		Short: "Shows help for tool cmd or a specific tools (only relates to builtin tools)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				// Show the general help for the tool command when no specific tool is mentioned
				return cmd.Help()
			}

			toolName := args[0]
			tool, ok := allTools[toolName]
			if !ok {
				return fmt.Errorf("tool %s not found", toolName)
			}

			// Display the detailed help text for the specific tool
			helpText := tool.HelpText()
			if helpText != "" {
				cmd.Println(helpText)
			} else {
				// Fallback to basic information if no specific help text is available
				cmd.Printf("Help for tool '%s'\n\n", toolName)
				cmd.Printf("Description: %s\n\n", tool.Description())
				cmd.Println("Input Schema:")
				cmd.Printf("%v\n", tool.InputSchema())
			}
			return nil
		},
	}

	// Add exec subcommand
	execCmd := &cobra.Command{
		Use:   "exec [toolName] [query...]",
		Short: "Executes a query in the specified tool",
		Long: `Executes a query in the specified tool.
		
Usage requires at least two positional arguments:
  1. toolName: The name of the tool to use
  2. query: One or more words forming the query to send to the tool`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// If no arguments, show help
			if len(args) == 0 {
				cmd.Help()
				return nil
			}

			// This is a direct tool execution, treat the first arg as a tool name
			// and the rest as the query
			if len(args) < 2 {
				return fmt.Errorf("not enough arguments, expected: tool [tool-name] [query]")
			}

			toolName := args[0]
			query := strings.Join(args[1:], " ")

			// Check if the tool exists
			tool, ok := allTools[toolName]
			if !ok {
				return fmt.Errorf("tool %s not found", toolName)
			}

			if len(args) < 2 {
				return fmt.Errorf("query required for executing tool %s", toolName)
			}

			// Execute the tool with the query
			jsonQuery := make(map[string]any)
			json.Unmarshal([]byte(query), &jsonQuery)

			result, err := tool.RunSchema(jsonQuery)
			if err != nil {
				return fmt.Errorf("failed to execute tool %s: %w", toolName, err)
			}

			result = handleMarkdown(result)
			cmd.Println(result)
			return nil
		},
	}

	// Add subcommands to the main command
	cmd.AddCommand(listCmd, helpCmd, execCmd)

	return cmd
}
