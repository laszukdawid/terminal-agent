package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/memory"
	"github.com/spf13/cobra"
)

var (
	memoryDir  = filepath.Join(os.Getenv("HOME"), ".local", "share", "terminal-agent")
	memoryFile = "memory.jsonl"
)

func getMemoryPath() string {
	return filepath.Join(memoryDir, memoryFile)
}

func NewMemoryCommand() *cobra.Command {
	mClient := memory.NewMemory(getMemoryPath())

	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Store and retrieve things to remember",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	cmd.AddCommand(memoryAddCommand(mClient))
	cmd.AddCommand(memoryListCommand(mClient))

	return cmd
}

func memoryAddCommand(mClient memory.Memory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add an entry to memory",
		Long: `Add an entry to memory. If the entry already exists, it will be silently skipped.

For example:
  agent memory add "viewing images is with catimg"
  agent memory add use kubectl for kubernetes`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			content := strings.Join(args, " ")

			if err := mClient.Add(content); err != nil {
				return fmt.Errorf("failed to add memory: %w", err)
			}

			return nil
		},
	}

	return cmd
}

func memoryListCommand(mClient memory.Memory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all memory entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := mClient.List()
			if err != nil {
				return fmt.Errorf("failed to list memory: %w", err)
			}

			if len(entries) == 0 {
				cmd.Println("No memory entries found.")
				return nil
			}

			for _, entry := range entries {
				cmd.Printf("%s %s\n", entry.Timestamp, entry.Content)
			}

			return nil
		},
	}

	return cmd
}
