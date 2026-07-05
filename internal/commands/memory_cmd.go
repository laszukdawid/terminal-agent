package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/memory"
	"github.com/laszukdawid/terminal-agent/internal/taskmemory"
	"github.com/spf13/cobra"
)

func getMemoryPath() string {
	return app.MemoryPath()
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
	cmd.AddCommand(taskMemoryCommand())

	return cmd
}

func taskMemoryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Search and manage indexed task memories",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
	cmd.AddCommand(taskMemorySearchCommand())
	cmd.AddCommand(taskMemoryStatsCommand())
	cmd.AddCommand(taskMemoryClearCommand())
	return cmd
}

func openTaskMemoryStore() (*taskmemory.SQLiteStore, error) {
	return taskmemory.Open(app.TaskMemoryPath())
}

func taskMemorySearchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search indexed task memories",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openTaskMemoryStore()
			if err != nil {
				return err
			}
			defer store.Close()
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Second)
			defer cancel()
			env := taskmemory.CurrentEnvironment("")
			matches, err := store.Search(ctx, taskmemory.Query{Text: strings.Join(args, " "), OS: env.OS, Arch: env.Arch, Shell: env.Shell, Limit: 10})
			if err != nil {
				return err
			}
			cmd.Println(taskmemory.FormatMatches(matches))
			return nil
		},
	}
	return cmd
}

func taskMemoryStatsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show task memory index statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openTaskMemoryStore()
			if err != nil {
				return err
			}
			defer store.Close()
			stats, err := store.Stats(cmd.Context())
			if err != nil {
				return err
			}
			cmd.Printf("Records: %d\n", stats.Records)
			cmd.Printf("FTS enabled: %t\n", stats.FTSEnabled)
			cmd.Printf("Path: %s\n", app.TaskMemoryPath())
			return nil
		},
	}
}

func taskMemoryClearCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Clear the task memory index",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openTaskMemoryStore()
			if err != nil {
				return err
			}
			defer store.Close()
			if err := store.Clear(cmd.Context()); err != nil {
				return err
			}
			cmd.Println("Task memory index cleared.")
			return nil
		},
	}
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
