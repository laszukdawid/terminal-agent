package commands

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/daemon"
	"github.com/spf13/cobra"
)

// NewDaemonCommand builds the `agent daemon` command group that supervises
// scheduled routine execution.
func NewDaemonCommand(cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "daemon",
		Short:        "Run and manage the routine scheduler daemon",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(daemonStartCommand(cfg))
	cmd.AddCommand(daemonStopCommand())
	cmd.AddCommand(daemonStatusCommand(cfg))
	cmd.AddCommand(daemonServiceCommand(true))
	cmd.AddCommand(daemonServiceCommand(false))
	return cmd
}

func daemonStartCommand(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:          "start",
		Short:        "Run the scheduler in the foreground (used by the service manager)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := daemon.New(cfg)
			if err != nil {
				return err
			}
			if err := d.Run(cmd.Context()); err != nil {
				if errors.Is(err, daemon.ErrAlreadyRunning) {
					return fmt.Errorf("a routine daemon is already running")
				}
				return err
			}
			return nil
		},
	}
}

func daemonStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "stop",
		Short:        "Stop the running scheduler daemon",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := daemon.Stop(); err != nil {
				if errors.Is(err, daemon.ErrNotRunning) {
					cmd.Println("No routine daemon is running.")
					return nil
				}
				return err
			}
			cmd.Println("Stop signal sent to the routine daemon.")
			return nil
		},
	}
}

func daemonStatusCommand(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:          "status",
		Short:        "Show daemon status and scheduled routines",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := daemon.CurrentStatus()
			if err != nil {
				return err
			}
			if status.Running {
				cmd.Printf("Daemon: running (pid %d)\n", status.PID)
			} else {
				cmd.Println("Daemon: stopped")
			}
			if !cfg.GetRoutinesEnabled() {
				cmd.Println("Routines are globally disabled (routines.enabled = false).")
			}

			views, err := newRoutineService(cfg).List(cmd.Context())
			if err != nil {
				return err
			}
			scheduled := 0
			var nextFire time.Time
			for _, v := range views {
				if !v.Routine.Enabled || strings.TrimSpace(v.Routine.Schedule) == "" {
					continue
				}
				scheduled++
				if next := v.Run.NextRunAt; !next.IsZero() && (nextFire.IsZero() || next.Before(nextFire)) {
					nextFire = next
				}
			}
			cmd.Printf("Scheduled routines: %d\n", scheduled)
			if !nextFire.IsZero() {
				cmd.Printf("Next fire: %s\n", nextFire.Local().Format("2006-01-02 15:04"))
			}
			return nil
		},
	}
}

func daemonServiceCommand(install bool) *cobra.Command {
	use := "uninstall"
	short := "Remove the daemon from the OS service manager"
	if install {
		use = "install"
		short = "Register the daemon with the OS service manager (launchd/systemd) to start on login"
	}
	return &cobra.Command{
		Use:          use,
		Short:        short,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			manager, err := daemon.NewServiceManager(exe)
			if err != nil {
				return err
			}
			if install {
				if err := manager.Install(); err != nil {
					return err
				}
				cmd.Println("Routine daemon installed and started.")
				return nil
			}
			if err := manager.Uninstall(); err != nil {
				return err
			}
			cmd.Println("Routine daemon service removed.")
			return nil
		},
	}
}
