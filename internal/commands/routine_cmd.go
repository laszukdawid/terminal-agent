package commands

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/app"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/daemon"
	"github.com/laszukdawid/terminal-agent/internal/routines"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// newRoutineService is a package var so tests can substitute a fake.
var newRoutineService = app.NewRoutineService

// NewRoutineCommand builds the `agent routine` command group.
func NewRoutineCommand(cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "routine",
		Short:        "Define and run scheduled, unattended agent routines",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(routineCreateCommand(cfg))
	cmd.AddCommand(routineListCommand(cfg))
	cmd.AddCommand(routineShowCommand(cfg))
	cmd.AddCommand(routineRunCommand(cfg))
	cmd.AddCommand(routineEnableCommand(cfg, true))
	cmd.AddCommand(routineEnableCommand(cfg, false))
	cmd.AddCommand(routineDeleteCommand(cfg))
	cmd.AddCommand(routineLogsCommand(cfg))

	return cmd
}

func routineCreateCommand(cfg config.Config) *cobra.Command {
	var (
		name, promptFlag, promptFile, cron string
		provider, model, timeout, workdir  string
		id                                 string
		toolsFlag, denyFlag                []string
		tokenBudget, maxTurns, maxToolCall int
		disabled                           bool
	)

	cmd := &cobra.Command{
		Use:          "create [prompt...]",
		Short:        "Create a routine",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt, err := resolveRoutinePrompt(promptFlag, promptFile, args)
			if err != nil {
				return err
			}
			flags := cmd.Flags()
			routine := routines.Routine{
				ID:         strings.TrimSpace(id),
				Name:       strings.TrimSpace(name),
				Prompt:     prompt,
				Schedule:   strings.TrimSpace(cron),
				Provider:   strings.TrimSpace(provider),
				Model:      strings.TrimSpace(model),
				Timeout:    strings.TrimSpace(timeout),
				WorkingDir: strings.TrimSpace(workdir),
				Tools:      toolsFlag,
				Deny:       denyFlag,
				Enabled:    !disabled,
			}
			if flags.Changed("token-budget") {
				routine.TokenBudget = &tokenBudget
			}
			if flags.Changed("max-turns") {
				routine.MaxTurns = &maxTurns
			}
			if flags.Changed("max-tool-calls") {
				routine.MaxToolCalls = &maxToolCall
			}

			saved, err := newRoutineService(cfg).Create(cmd.Context(), routine)
			if err != nil {
				return err
			}
			cmd.Printf("Created routine %q.\n", saved.ID)
			if saved.Schedule == "" {
				cmd.Println("No schedule set; run it with `agent routine run " + saved.ID + "`.")
			} else {
				offerDaemonInstall(cmd)
			}
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "human-readable routine name")
	flags.StringVar(&id, "id", "", "explicit routine id (derived from name when omitted)")
	flags.StringVarP(&promptFlag, "prompt", "p", "", "routine prompt")
	flags.StringVar(&promptFile, "prompt-file", "", "read the prompt from a file")
	flags.StringVar(&cron, "cron", "", "cron schedule (empty = manual only)")
	flags.StringVar(&provider, "provider", "", "provider override")
	flags.StringVar(&model, "model", "", "model override")
	flags.StringVar(&timeout, "timeout", "", "wall-clock timeout (Go duration; 0 = unlimited)")
	flags.IntVar(&tokenBudget, "token-budget", 0, "token budget (0 = unlimited)")
	flags.IntVar(&maxTurns, "max-turns", 0, "max turns")
	flags.IntVar(&maxToolCall, "max-tool-calls", 0, "max tool calls")
	flags.StringVar(&workdir, "workdir", "", "working directory for the run")
	flags.StringSliceVar(&toolsFlag, "tools", nil, "enabled tools (default policy disables external-facing tools)")
	flags.StringSliceVar(&denyFlag, "deny", nil, "routine-scoped deny rules")
	flags.BoolVar(&disabled, "disabled", false, "create the routine disabled")
	return cmd
}

func routineListCommand(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:          "list",
		Short:        "List routines",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			views, err := newRoutineService(cfg).List(cmd.Context())
			if err != nil {
				return err
			}
			if len(views) == 0 {
				cmd.Println("No routines defined. Create one with `agent routine create`.")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSTATUS\tSCHEDULE\tLAST\tNEXT\tMODEL\tPROMPT")
			for _, v := range views {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					v.Routine.ID,
					v.Status,
					v.Frequency,
					formatRoutineTime(v.Run.LastRunAt),
					formatRoutineTime(v.Run.NextRunAt),
					orDefault(v.Routine.Model),
					v.Routine.PromptPreview(60),
				)
			}
			return w.Flush()
		},
	}
}

func routineShowCommand(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:          "show <id>",
		Short:        "Show a routine's full definition and last run",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := newRoutineService(cfg).Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			r := v.Routine
			cmd.Printf("ID:        %s\n", r.ID)
			cmd.Printf("Name:      %s\n", orDefault(r.Name))
			cmd.Printf("Status:    %s\n", v.Status)
			cmd.Printf("Schedule:  %s\n", v.Frequency)
			cmd.Printf("Provider:  %s\n", orDefault(r.Provider))
			cmd.Printf("Model:     %s\n", orDefault(r.Model))
			cmd.Printf("Timeout:   %s\n", orDefault(r.Timeout))
			cmd.Printf("Tools:     %s\n", formatToolPolicy(r.Tools))
			if len(r.Deny) > 0 {
				cmd.Printf("Deny:      %s\n", strings.Join(r.Deny, ", "))
			}
			if v.HasRun {
				cmd.Printf("Last run:  %s (%s)\n", formatRoutineTime(v.Run.LastRunAt), v.Run.LastStatus)
				if v.Run.LastError != "" {
					cmd.Printf("Last error: %s\n", v.Run.LastError)
				}
			}
			cmd.Printf("\nPrompt:\n%s\n", r.Prompt)
			return nil
		},
	}
}

func routineRunCommand(cfg config.Config) *cobra.Command {
	var scheduled bool
	var print bool
	cmd := &cobra.Command{
		Use:          "run <id>",
		Short:        "Run a routine now",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			trigger := routines.TriggerManual
			if scheduled {
				trigger = routines.TriggerScheduled
			}
			result, err := newRoutineService(cfg).Run(cmd.Context(), app.RoutineRunRequest{
				IDOrName: args[0],
				Trigger:  trigger,
			})
			if errors.Is(err, routines.ErrRunInProgress) {
				cmd.Printf("Routine %q is already running; skipped.\n", args[0])
				return nil
			}
			if err != nil {
				return err
			}
			if print && strings.TrimSpace(result.Output) != "" {
				cmd.Println(strings.TrimSpace(result.Output))
			}
			if result.Err != nil {
				return fmt.Errorf("routine %s %s: %w", result.Routine.ID, result.Outcome, result.Err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&scheduled, "scheduled", false, "mark this as a scheduler-originated run")
	cmd.Flags().BoolVar(&print, "print", true, "print the routine's final output")
	_ = cmd.Flags().MarkHidden("scheduled")
	return cmd
}

func routineEnableCommand(cfg config.Config, enable bool) *cobra.Command {
	use := "disable <id>"
	short := "Disable a routine"
	if enable {
		use = "enable <id>"
		short = "Enable a routine"
	}
	return &cobra.Command{
		Use:          use,
		Short:        short,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := newRoutineService(cfg)
			v, err := svc.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			routine := v.Routine
			routine.Enabled = enable
			if _, err := svc.Save(cmd.Context(), routine); err != nil {
				return err
			}
			state := "disabled"
			if enable {
				state = "enabled"
			}
			cmd.Printf("Routine %q %s.\n", routine.ID, state)
			return nil
		},
	}
}

func routineDeleteCommand(cfg config.Config) *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:          "delete <id>",
		Short:        "Delete a routine",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := newRoutineService(cfg).Delete(cmd.Context(), args[0], purge); err != nil {
				return err
			}
			cmd.Printf("Deleted routine %q.\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "also remove stored run logs and results")
	return cmd
}

func routineLogsCommand(cfg config.Config) *cobra.Command {
	var last bool
	cmd := &cobra.Command{
		Use:          "logs <id>",
		Short:        "List a routine's run logs, or print the latest summary",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			refs, err := newRoutineService(cfg).Logs(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if len(refs) == 0 {
				cmd.Println("No runs recorded yet.")
				return nil
			}
			if last {
				for _, ref := range refs {
					if ref.IsResult {
						content, err := os.ReadFile(ref.Path)
						if err != nil {
							return err
						}
						cmd.Print(string(content))
						return nil
					}
				}
				cmd.Println("No result summary found; see the transcript files below.")
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "WHEN\tTYPE\tFILE")
			for _, ref := range refs {
				kind := "transcript"
				if ref.IsResult {
					kind = "summary"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", formatRoutineTime(ref.ModTime), kind, ref.Path)
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&last, "last", false, "print the latest result summary")
	return cmd
}

// PrintRoutineLaunchNotice prints a one-line summary of routine runs that
// completed since the user was last here, when invoked from an interactive
// terminal and not from within the routine/daemon command subtrees.
func PrintRoutineLaunchNotice(cmd *cobra.Command) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return
	}
	path := cmd.CommandPath()
	if strings.Contains(path, "routine") || strings.Contains(path, "daemon") {
		return
	}
	cfg, err := config.LoadConfig()
	if err != nil || !cfg.GetRoutinesEnabled() {
		return
	}
	notice, err := newRoutineService(cfg).LaunchNotice(cmd.Context())
	if err != nil || notice == "" {
		return
	}
	fmt.Fprintln(cmd.ErrOrStderr(), notice)
}

// offerDaemonInstall nudges the user to set up the scheduler the first time they
// create a scheduled routine. If a daemon is already running there is nothing to
// do; otherwise it offers (interactively) to install and start it, or prints how
// to do it manually when not attached to a terminal.
func offerDaemonInstall(cmd *cobra.Command) {
	if status, err := daemon.CurrentStatus(); err == nil && status.Running {
		return // the scheduler is already running; the routine will fire
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		cmd.Println("The scheduler isn't running, so scheduled routines won't fire. Start it with `agent daemon install`.")
		return
	}
	cmd.Print("The scheduler isn't running, so this scheduled routine won't fire yet. Install and start it now? [y/N]: ")
	answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if !isAffirmative(answer) {
		cmd.Println("Skipped. Run `agent daemon install` to enable scheduled runs (and `agent daemon uninstall` to remove it).")
		return
	}
	exe, err := os.Executable()
	if err != nil {
		cmd.Printf("Could not resolve the agent binary: %v. Run `agent daemon install` manually.\n", err)
		return
	}
	manager, err := daemon.NewServiceManager(exe)
	if err != nil {
		cmd.Printf("Could not prepare the service manager: %v. Run `agent daemon install` manually.\n", err)
		return
	}
	if err := manager.Install(); err != nil {
		cmd.Printf("Install failed: %v. You can run `agent daemon install` manually.\n", err)
		return
	}
	cmd.Println("Scheduler installed and started. Manage it with `agent daemon status | stop | uninstall`.")
}

func isAffirmative(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

func resolveRoutinePrompt(promptFlag, promptFile string, args []string) (string, error) {
	if strings.TrimSpace(promptFlag) != "" {
		return promptFlag, nil
	}
	if strings.TrimSpace(promptFile) != "" {
		content, err := os.ReadFile(promptFile)
		if err != nil {
			return "", fmt.Errorf("read prompt file: %w", err)
		}
		return strings.TrimSpace(string(content)), nil
	}
	if joined := strings.TrimSpace(strings.Join(args, " ")); joined != "" {
		return joined, nil
	}
	return "", fmt.Errorf("a prompt is required (use --prompt, --prompt-file, or positional args)")
}

func formatRoutineTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func orDefault(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(default)"
	}
	return value
}

func formatToolPolicy(toolsList []string) string {
	if toolsList == nil {
		return "default (external-facing disabled)"
	}
	if len(toolsList) == 0 {
		return "(none)"
	}
	return strings.Join(toolsList, ", ")
}
