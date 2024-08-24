package main

import (
	"context"
	"fmt"
	"os"

	"github.com/laszukdawid/terminal-agent/internal/commands/ask"
	"github.com/laszukdawid/terminal-agent/internal/commands/task"
	u "github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var loglevel string

type exitCode int

const (
	exitOk exitCode = iota
	exitNotOk
)

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

	return cmd
}

func main() {
	code := mainRun()
	os.Exit(int(code))
}

func mainRun() exitCode {
	// Define flags
	cmd := NewCommand()
	cmd.AddCommand(ask.NewQuestionCommand())
	cmd.AddCommand(task.NewTaskCommand())

	ctx := context.Background()

	// Execute the command
	if err := cmd.ExecuteContext(ctx); err != nil {
		logger := u.GetLogger()
		logger.Error("Command execution failed", zap.Error(err))
		return exitNotOk
	}

	return exitOk
}
