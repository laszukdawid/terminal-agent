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

	return cmd
}

func main() {
	code := mainRun()
	os.Exit(int(code))
}

func mainRun() exitCode {

	logger := u.InitLogger()
	defer logger.Sync()

	// Define flags
	cmd := NewCommand()
	cmd.AddCommand(ask.NewQuestionCommand())
	cmd.AddCommand(task.NewTaskCommand())

	ctx := context.Background()

	// Execute the command
	if err := cmd.ExecuteContext(ctx); err != nil {
		logger.Error("Command execution failed", zap.Error(err))
		return exitNotOk
	}

	return exitOk
}
