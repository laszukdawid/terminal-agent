package commands

import (
	"fmt"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/spf13/cobra"
)

func ConfigCommand(config config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage the terminal-agent configuration",
		Long:  `Manage the terminal-agent configuration. You can set the log level, default provider, and default model ID.`,
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
			case "provider":
				fmt.Println(config.GetDefaultProvider())
			case "model":
				fmt.Println(config.GetDefaultModelId())
			default:
				fmt.Println("Unknown key:", key)
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
		Long: `Set the configuration. Needs two values: key and value. Currently supported keys: provider, model.

For example: terminal-agent config set provider bedrock`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) < 2 {
				cmd.Help()
				return
			}

			key := args[0]
			value := args[1]

			switch key {
			case "provider":
				config.SetDefaultProvider(value)
				fmt.Println("Default provider set to:", value)
			case "model":
				config.SetDefaultModelId(value)
				fmt.Println("Default model ID set to:", value)
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
			fmt.Println("Default provider:", config.GetDefaultProvider())
			fmt.Println("Default model ID:", config.GetDefaultModelId())
		},
	}

	return cmd
}
