package commands

import (
	"os"
	"path/filepath"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/history"
	"github.com/spf13/cobra"
)

var (
	logDir  = filepath.Join(os.Getenv("HOME"), ".local", "share", "terminal-agent")
	logFile = "query_log.jsonl"
)

func getLogPath() string {
	return filepath.Join(logDir, logFile)
}

func NewHistoryCommand(config config.Config) *cobra.Command {
	hClient := history.NewHistory(getLogPath())

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Query interaction history",
		Long: `Query interaction history

		When using --after and --before flags, the date can be in any of these formats:
		YYYY, YYYY-MM-DD,  HH:MM:SS, YYYY-MM-DDTHH:MM:SS, YYYY-MM-DDTHH:MM:SSZ

		When specifying only time it's implicitly today. When specifying only date it's implicitly midnight.
		Incorrect time values will be ignored.

		Any remaining argument that isn't captured by the flags will be concatenated to form the query.`,
		RunE: func(cmd *cobra.Command, args []string) error {

			flags := cmd.Flags()
			afterStr, _ := flags.GetString("after")
			beforeStr, _ := flags.GetString("before")

			query := history.HistoryQuery{
				AfterStr:  afterStr,
				BeforeStr: beforeStr,
			}
			if logs, err := hClient.Query(query); err == nil {
				for _, log := range logs {
					cmd.Println(log)
				}
			} else {
				return err
			}
			return nil
		},
	}

	cmd.MarkFlagRequired("history")

	cmd.Flags().String("after", "", "Filter logs after this date")
	cmd.Flags().String("before", "", "Filter logs before this date")

	return cmd
}
