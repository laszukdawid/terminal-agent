package main

import (
	"fmt"
	"os"

	"github.com/laszukdawid/terminal-agent/internal/commands/ask"
	"github.com/spf13/cobra"
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
	// Define flags
	cmd := NewCommand()
	cmd.AddCommand(ask.NewQuestionCommand())

	// Execute the command
	if err := cmd.Execute(); err != nil {
		err = fmt.Errorf("execution failed: %w", err)
		fmt.Printf("Error: %v\n", err)
		return exitNotOk
	}

	return exitOk
}
