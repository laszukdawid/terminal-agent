package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/laszukdawid/terminal-agent/internal/commands"
	"github.com/laszukdawid/terminal-agent/internal/config"
	u "github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	loglevel string
)

type exitCode int

const (
	exitOk exitCode = iota
	exitNotOk
)

// printVersion prints the version of the CLI
// The version is based on the golang module version which is based on git tag
func printVersion() {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		fmt.Println("Unable to determine version information.")
		return
	}

	if buildInfo.Main.Version != "" {
		fmt.Printf("Version: %s\n", buildInfo.Main.Version)
	} else {
		fmt.Println("Version: unknown")
	}
}

// NewCommand creates a new cobra command
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Terminal Agent is a CLI tool to interact with the terminal",
		Long:  `Terminal Agent is a CLI tool to interact with the terminal. It can be used to run commands, ask questions, and more.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logger, err := u.InitLogger(&loglevel)

			if err != nil {
				fmt.Println("Failed to initialize logger")
				os.Exit(1)
			}
			defer logger.Sync()

		},
		Run: func(cmd *cobra.Command, args []string) {
			u.Logger.Debug("Running command", zap.Strings("args", args))

			// Check if version flag is set
			if versionFlag, _ := cmd.Flags().GetBool("version"); versionFlag {
				printVersion()
				return
			}

			if len(args) == 0 {
				cmd.Help()
				// return exitNotOk
			} else {
				fmt.Println("Welcome to Terminal Agent CLI!")
				fmt.Println("Running command: ", args)
			}
		},
	}
	cmd.PersistentFlags().StringVar(&loglevel, "loglevel", "info", "set the log level (debug, info, warn, error, dpanic, panic, fatal)")
	cmd.Flags().BoolP("version", "v", false, "Print the version of the CLI")

	return cmd
}

func main() {
	code := mainRun()
	os.Exit(int(code))
}

func mainRun() exitCode {

	// Load configuration for each execution
	c, err := config.LoadConfig()
	if err != nil {
		c = config.NewDefaultConfig()
	}

	// Define flags
	cmd := NewCommand()
	cmd.AddCommand(commands.NewQuestionCommand(c))
	cmd.AddCommand(commands.NewChatCommand(c))
	cmd.AddCommand(commands.NewHistoryCommand(c))
	cmd.AddCommand(commands.NewConfigCommand(c))
	cmd.AddCommand(commands.NewToolCommand(c))
	cmd.AddCommand(commands.NewTaskCommand(c))
	cmd.AddCommand(commands.NewMemoryCommand())

	ctx := context.Background()

	// Execute the command
	if err := cmd.ExecuteContext(ctx); err != nil {
		return exitNotOk
	}

	return exitOk
}
