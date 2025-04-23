package commands

import (
	"fmt"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/spf13/cobra"
)

func NewToolCommand(config config.Config) *cobra.Command {
	allTools := tools.GetAllTools()

	cmd := &cobra.Command{
		Use:          "tool",
		SilenceUsage: true,
		Short:        "Manage and execute tools",
		Long: `Manage and execute tools.

Available subcommands:
  - list: Lists all available tools
  - help: Shows help information for specific tools
  - exec: Executes a query in the specified tool`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// When no arguments or subcommands are provided, print help
			return cmd.Help()
		},
	}

	// Add list subcommand
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Lists all available tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			for name := range allTools {
				fmt.Println(name)
			}
			return nil
		},
	}

	// Add help subcommand
	helpCmd := &cobra.Command{
		Use:   "help [toolName]",
		Short: "Shows help for tool cmd or a specific tools",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				// Show the general help for the tool command when no specific tool is mentioned
				return cmd.Help()
			}

			toolName := args[0]
			if _, ok := allTools[toolName]; !ok {
				return fmt.Errorf("tool %s not found", toolName)
			}

			fmt.Printf("Help for tool '%s'\n", toolName)
			// Here you can add more specific help for each tool if available
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
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			toolName := args[0]

			// Check if the tool exists
			if _, ok := allTools[toolName]; !ok {
				return fmt.Errorf("tool %s not found", toolName)
			}

			if len(args) < 2 {
				return fmt.Errorf("query required for executing tool %s", toolName)
			}

			query := strings.Join(args[1:], " ")
			tool, ok := allTools[toolName]

			fmt.Println("Tool name:", toolName)
			fmt.Println("Query:", query)

			if !ok {
				return fmt.Errorf("tool %s not found", toolName)
			}

			// Execute the tool with the query
			result, err := tool.Run(&query)
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
