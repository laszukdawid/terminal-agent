package commands

import (
	"fmt"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/spf13/cobra"
)

const (
	cmdProvider = "provider"
	cmdModel    = "model"
	cmdMcpPath  = "mcp-path"
)

func NewConfigCommand(config config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage the terminal-agent configuration",
		Long:  `Manage the terminal-agent configuration. You can set the log level, default provider, default model ID, and MCP file path.`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	cmd.AddCommand(ConfigSetCommand(config))
	cmd.AddCommand(ConfigGetCommand(config))

	return cmd
}

func ConfigGetCommand(config config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get the current configuration",
		Run: func(cmd *cobra.Command, args []string) {
			// Check if there are any arguments
			if len(args) != 1 {
				cmd.Help()
				return
			}

			key := args[0]
			switch key {
			case cmdProvider:
				fmt.Println(config.GetDefaultProvider())
			case cmdModel:
				fmt.Println(config.GetDefaultModelId())
			case cmdMcpPath:
				fmt.Println(config.GetMcpFilePath())
			default:
				fmt.Println("Unknown key:", key)
				cmd.Help()
			}
		},
	}

	cmd.AddCommand(ConfigGetAll(config))

	return cmd
}

func ConfigSetCommand(config config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set the configuration",
		Long: `Set the configuration. Needs two values: key and value. Currently supported keys: provider, model, mcp-path.

For example: 
  terminal-agent config set provider bedrock
  terminal-agent config set mcp-path /path/to/mcp.json`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) < 2 {
				cmd.Help()
				return
			}

			key := args[0]
			value := args[1]

			switch key {
			case cmdProvider:
				config.SetDefaultProvider(value)
				fmt.Println("Default provider set to:", value)
			case cmdModel:
				config.SetDefaultModelId(value)
				fmt.Println("Default model ID set to:", value)
			case cmdMcpPath:
				config.SetMcpFilePath(value)
				fmt.Println("MCP file path set to:", value)
			default:
				fmt.Println("Unknown key:", key)
			}
		},
	}

	return cmd
}

func ConfigGetAll(config config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "all",
		Short: "Get all configuration values",
		Run: func(cmd *cobra.Command, args []string) {
			// Print each key and value on a new line
			fmt.Println("Default provider: ", cmdProvider, "=", config.GetDefaultProvider())
			fmt.Println("Default model ID:", cmdModel, "=", config.GetDefaultModelId())
			fmt.Println("MCP file path:", cmdMcpPath, "=", config.GetMcpFilePath())
		},
	}

	return cmd
}
