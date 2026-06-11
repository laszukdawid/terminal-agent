package commands

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/spf13/cobra"
)

const (
	cmdProvider   = "provider"
	cmdModel      = "model"
	cmdMcpPath    = "mcp-path"
	cmdWorkingDir = "working-dir"
	cmdMemory     = "memory"
	cmdDevice     = "device"

	bedrockPriceRefreshTimeout = 3 * time.Second
)

type bedrockPriceCacheConfig interface {
	GetBedrockModelPrice(region string, modelID string) (inputPer1K, outputPer1K float64, lastChecked string, ok bool)
	SetBedrockModelPrice(region string, modelID string, inputPer1K, outputPer1K float64, lastChecked string) error
}

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
	cmd.AddCommand(ConfigShowAllCommand(config))

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
			case cmdWorkingDir:
				fmt.Println(config.GetWorkingDir())
			case cmdMemory:
				fmt.Println(config.GetMemory())
			case cmdDevice:
				fmt.Println(config.GetDevice())
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
		Long: `Set the configuration. Needs two values: key and value. Currently supported keys: provider, model, mcp-path, working-dir, memory, device.

For example:
  terminal-agent config set provider bedrock
  terminal-agent config set mcp-path /path/to/mcp.json
  terminal-agent config set working-dir /path/to/workdir
  terminal-agent config set memory true
  terminal-agent config set device cpu`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) < 2 {
				cmd.Help()
				return
			}

			key := args[0]
			value := args[1]

			switch key {
			case cmdProvider:
				supported := connector.SupportedProviders()
				if !slices.Contains(supported, value) {
					fmt.Printf("Unsupported provider: %s\n", value)
					fmt.Printf("Supported providers: %s\n", strings.Join(supported, ", "))
					return
				}
				if err := config.SetDefaultProvider(value); err != nil {
					cmd.SilenceUsage = true
					cmd.PrintErrln(err.Error())
					return
				}
				fmt.Println("Default provider set to:", value)
				if value == connector.BedrockProvider {
					refreshBedrockModelPriceIfNeeded(context.Background(), config, config.GetDefaultModelId(), time.Now(), cmd.ErrOrStderr())
				}
			case cmdModel:
				if err := config.SetDefaultModelId(value); err != nil {
					cmd.SilenceUsage = true
					cmd.PrintErrln(err.Error())
					return
				}
				fmt.Println("Default model ID set to:", value)
				if config.GetDefaultProvider() == connector.BedrockProvider {
					refreshBedrockModelPriceIfNeeded(context.Background(), config, value, time.Now(), cmd.ErrOrStderr())
				}
			case cmdMcpPath:
				config.SetMcpFilePath(value)
				fmt.Println("MCP file path set to:", value)
			case cmdWorkingDir:
				config.SetWorkingDir(value)
				fmt.Println("Working directory set to:", value)
			case cmdMemory:
				if value == "true" {
					config.SetMemory(true)
					fmt.Println("Memory set to: true")
				} else if value == "false" {
					config.SetMemory(false)
					fmt.Println("Memory set to: false")
				} else {
					cmd.PrintErrln("Invalid value for memory. Use 'true' or 'false'.")
					cmd.SilenceUsage = true
					_ = cmd.Help()
					return
				}
			case cmdDevice:
				if err := config.SetDevice(value); err != nil {
					cmd.SilenceUsage = true
					cmd.PrintErrln(err.Error())
					return
				}
				fmt.Println("Device set to:", config.GetDevice())
			default:
				cmd.SilenceUsage = true
				cmd.PrintErrln("Unknown key:", key)
				return
			}
		},
	}

	return cmd
}

func refreshBedrockModelPriceIfNeeded(ctx context.Context, cfg config.Config, modelID string, now time.Time, errWriter io.Writer) {
	ctx, cancel := context.WithTimeout(ctx, bedrockPriceRefreshTimeout)
	defer cancel()

	cache, ok := cfg.(bedrockPriceCacheConfig)
	if !ok {
		return
	}
	region := connector.ResolveBedrockRegion(ctx, cfg)
	if _, _, lastChecked, ok := cache.GetBedrockModelPrice(region, modelID); ok && !bedrockPriceCacheExpired(lastChecked, now) {
		return
	}

	price, err := connector.FetchBedrockModelPrice(ctx, cfg, connector.BedrockModelID(modelID))
	if err != nil {
		fmt.Fprintf(errWriter, "Warning: could not refresh Bedrock pricing for %s: %v\n", modelID, err)
		fmt.Fprintln(errWriter, "Cost estimates for this Bedrock model will be unavailable until pricing is configured or refreshed successfully.")
		return
	}

	checkedAt := now.UTC().Format(time.RFC3339)
	if err := cache.SetBedrockModelPrice(region, modelID, price.InputPer1K, price.OutputPer1K, checkedAt); err != nil {
		fmt.Fprintf(errWriter, "Warning: could not save Bedrock pricing for %s: %v\n", modelID, err)
	}
}

func bedrockPriceCacheExpired(lastChecked string, now time.Time) bool {
	checkedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(lastChecked))
	if err != nil {
		return true
	}
	return now.Sub(checkedAt) > 24*time.Hour
}

func ConfigShowAllCommand(config config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show-all",
		Short: "Show all configuration values",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Default provider:", cmdProvider, "=", config.GetDefaultProvider())
			fmt.Println("Default model ID:", cmdModel, "=", config.GetDefaultModelId())
			fmt.Println("Device:", cmdDevice, "=", config.GetDevice())
			fmt.Println("MCP file path:", cmdMcpPath, "=", config.GetMcpFilePath())
			fmt.Println("Working directory:", cmdWorkingDir, "=", config.GetWorkingDir())
			fmt.Println("Memory:", cmdMemory, "=", config.GetMemory())
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
			fmt.Println("Device:", cmdDevice, "=", config.GetDevice())
			fmt.Println("MCP file path:", cmdMcpPath, "=", config.GetMcpFilePath())
			fmt.Println("Working directory:", cmdWorkingDir, "=", config.GetWorkingDir())
			fmt.Println("Memory:", cmdMemory, "=", config.GetMemory())
		},
	}

	return cmd
}
