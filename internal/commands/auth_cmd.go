package commands

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/auth"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func NewAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "auth",
		Short:        "Manage provider authentication",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(authLoginCommand())
	cmd.AddCommand(authStatusCommand())
	cmd.AddCommand(authLogoutCommand())

	return cmd
}

func authLoginCommand() *cobra.Command {
	var device bool
	var apiKeyMode bool
	var apiKey string

	cmd := &cobra.Command{
		Use:          "login [provider]",
		Short:        "Authenticate with a provider",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := auth.NormalizeProvider(args[0])
			if err := auth.ValidateProvider(provider); err != nil {
				return err
			}

			if device && apiKeyMode {
				return fmt.Errorf("use only one auth mode flag")
			}
			if apiKeyMode && provider != auth.ProviderOpenAI {
				return fmt.Errorf("%s does not support API-key auth; use 'agent auth login openai --api-key'", provider)
			}
			if !apiKeyMode && provider != auth.ProviderCodex {
				return fmt.Errorf("%s does not support OAuth auth; use 'agent auth login codex'", provider)
			}

			manager := auth.NewManager()

			if device {
				cfg := auth.DefaultBrowserLoginConfig()
				cfg.OpenBrowser = false
				cmd.PrintErrln("Starting device login...")
				result, err := manager.LoginOpenAIDevice(cfg)
				if err != nil {
					return err
				}
				cmd.Printf("Successfully authenticated with Codex.\n")
				cmd.Printf("Account ID: %s\n", result.AccountID)
				cmd.Printf("Credentials stored in %s\n", manager.Path())
				return nil
			}

			if apiKeyMode {
				key, err := resolveAPIKeyInput(apiKey)
				if err != nil {
					return err
				}
				if err := manager.SaveAPIKey(provider, key); err != nil {
					return err
				}
				cmd.Printf("Stored OpenAI API key in %s\n", manager.Path())
				return nil
			}

			cfg := auth.DefaultBrowserLoginConfig()
			cfg.ManualCodeReader = os.Stdin
			cmd.PrintErrln("Starting browser login...")
			result, err := manager.LoginOpenAIBrowser(cfg)
			if err != nil {
				return err
			}

			cmd.Printf("Successfully authenticated with Codex.\n")
			cmd.Printf("Account ID: %s\n", result.AccountID)
			cmd.Printf("Credentials stored in %s\n", manager.Path())
			return nil
		},
	}

	cmd.Flags().BoolVar(&device, "device", false, "Use device-code login flow")
	cmd.Flags().BoolVar(&apiKeyMode, "api-key", false, "Store an API key instead of using OAuth")
	cmd.Flags().StringVar(&apiKey, "key", "", "API key to store (otherwise read from OPENAI_API_KEY, terminal prompt, or stdin)")

	return cmd
}

func authStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "status [provider]",
		Short:        "Show auth status for a provider",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := auth.NormalizeProvider(args[0])
			manager := auth.NewManager()

			status, err := manager.Status(provider)
			if err != nil {
				return err
			}

			configured := "no"
			if status.Configured {
				configured = "yes"
			}

			cmd.Printf("Provider: %s\n", status.Provider)
			cmd.Printf("Configured: %s\n", configured)
			cmd.Printf("Path: %s\n", status.Path)
			if !status.Configured {
				return nil
			}

			cmd.Printf("Auth type: %s\n", status.Type)
			cmd.Printf("Source: %s\n", status.Source)
			if status.AccountID != "" {
				cmd.Printf("Account ID: %s\n", status.AccountID)
			}
			if status.PlanType != "" {
				cmd.Printf("Plan type: %s\n", status.PlanType)
			}
			if !status.ExpiresAt.IsZero() {
				cmd.Printf("Expires: %s\n", status.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"))
				cmd.Printf("Expired: %t\n", status.Expired)
			}

			return nil
		},
	}

	return cmd
}

func authLogoutCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "logout [provider]",
		Short:        "Remove stored auth for a provider",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			provider := auth.NormalizeProvider(args[0])
			if err := auth.ValidateProvider(provider); err != nil {
				return err
			}

			manager := auth.NewManager()
			removed, err := manager.DeleteProvider(provider)
			if err != nil {
				return err
			}

			if !removed {
				cmd.Printf("No stored auth found for %s\n", provider)
				return nil
			}

			cmd.Printf("Removed stored auth for %s\n", provider)
			return nil
		},
	}

	return cmd
}

func resolveAPIKeyInput(flagValue string) (string, error) {
	trimmedFlagValue := strings.TrimSpace(flagValue)
	if trimmedFlagValue != "" {
		return trimmedFlagValue, nil
	}

	trimmedEnvValue := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if trimmedEnvValue != "" {
		return trimmedEnvValue, nil
	}

	stdinFD := int(os.Stdin.Fd())
	if term.IsTerminal(stdinFD) {
		fmt.Fprint(os.Stderr, "OpenAI API key: ")
		value, err := term.ReadPassword(stdinFD)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("failed to read API key from terminal: %w", err)
		}
		trimmed := strings.TrimSpace(string(value))
		if trimmed == "" {
			return "", fmt.Errorf("API key cannot be empty")
		}
		return trimmed, nil
	}

	value, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("failed to read API key from stdin: %w", err)
	}

	trimmed := strings.TrimSpace(string(value))
	if trimmed == "" {
		return "", fmt.Errorf("API key cannot be empty")
	}
	return trimmed, nil
}
