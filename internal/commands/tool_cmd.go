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
		Args:         cobra.MinimumNArgs(2),
		Short:        "Executes a query in the specified tool",
		Long: `Executes a query in the specified tool.
		The first argument is the tool name, and the rest of the arguments form the query.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolName := args[0]
			query := strings.Join(args[1:], " ")

			var result string

			// Check if the tool exists
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

			cmd.Println(result)
			return nil
		},
	}

	return cmd
}
