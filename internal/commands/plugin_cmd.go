package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	bashReaderPluginName  = "bash-reader"
	bashReaderBlockStart  = "# BEGIN terminal-agent bash-reader"
	bashReaderBlockEnd    = "# END terminal-agent bash-reader"
	bashReaderSourceLine  = "source \"$HOME/.config/terminal-agent/plugins/bash-reader/init.bash\""
	bashReaderInstallHint = "agent plugin install bash-reader"
)

func NewPluginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage terminal-agent plugins",
		Long:  "Manage terminal-agent plugins, including shell integrations.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	installCmd := &cobra.Command{
		Use:   "install [plugin-name]",
		Short: "Install a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to determine user home directory: %w", err)
			}

			pluginName := strings.TrimSpace(args[0])
			switch pluginName {
			case bashReaderPluginName:
				result, err := installBashReaderPlugin(homeDir)
				if err != nil {
					return err
				}

				cmd.Printf("Installed %s at: %s\n", bashReaderPluginName, result.ScriptPath)
				if result.BashRCUpdated {
					cmd.Printf("Updated %s to source the plugin.\n", result.BashRCPath)
				} else {
					cmd.Printf("%s already sources the plugin.\n", result.BashRCPath)
				}
				cmd.Println("Restart your shell or run: source ~/.bashrc")
				return nil
			default:
				return fmt.Errorf("unsupported plugin %q. Available plugins: %s", pluginName, bashReaderPluginName)
			}
		},
	}

	var purgeData bool
	uninstallCmd := &cobra.Command{
		Use:   "uninstall [plugin-name]",
		Short: "Uninstall a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to determine user home directory: %w", err)
			}

			pluginName := strings.TrimSpace(args[0])
			switch pluginName {
			case bashReaderPluginName:
				result, err := uninstallBashReaderPlugin(homeDir, purgeData)
				if err != nil {
					return err
				}

				if result.BashRCUpdated {
					cmd.Printf("Updated %s and removed bash-reader block.\n", result.BashRCPath)
				} else {
					cmd.Printf("No bash-reader block found in %s.\n", result.BashRCPath)
				}

				if result.PluginDirRemoved {
					cmd.Printf("Removed plugin directory: %s\n", result.PluginDirPath)
				} else {
					cmd.Printf("Plugin directory not found: %s\n", result.PluginDirPath)
				}

				if purgeData {
					if result.ContextDirRemoved {
						cmd.Printf("Removed terminal context data: %s\n", result.ContextDirPath)
					} else {
						cmd.Printf("Terminal context data not found: %s\n", result.ContextDirPath)
					}
				}

				cmd.Println("Restart your shell or run: source ~/.bashrc")
				return nil
			default:
				return fmt.Errorf("unsupported plugin %q. Available plugins: %s", pluginName, bashReaderPluginName)
			}
		},
	}
	uninstallCmd.Flags().BoolVar(&purgeData, "purge-data", false, "Remove captured terminal context data for the plugin")

	cmd.AddCommand(installCmd, uninstallCmd)
	return cmd
}

type bashReaderInstallResult struct {
	ScriptPath    string
	BashRCPath    string
	BashRCUpdated bool
}

type bashReaderUninstallResult struct {
	BashRCPath        string
	BashRCUpdated     bool
	PluginDirPath     string
	PluginDirRemoved  bool
	ContextDirPath    string
	ContextDirRemoved bool
}

func installBashReaderPlugin(homeDir string) (bashReaderInstallResult, error) {
	scriptPath := getBashReaderScriptPath(homeDir)
	bashRCPath := getBashRCPath(homeDir)
	contextDir := getTerminalContextDir(homeDir)

	if err := os.MkdirAll(filepath.Dir(scriptPath), 0755); err != nil {
		return bashReaderInstallResult{}, fmt.Errorf("failed to create plugin directory: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(contextDir, "sessions"), 0755); err != nil {
		return bashReaderInstallResult{}, fmt.Errorf("failed to create terminal context directory: %w", err)
	}

	if err := os.WriteFile(scriptPath, []byte(bashReaderInitScript), 0644); err != nil {
		return bashReaderInstallResult{}, fmt.Errorf("failed to write plugin script: %w", err)
	}

	bashRCUpdated, err := ensureBashReaderSourceBlock(bashRCPath)
	if err != nil {
		return bashReaderInstallResult{}, err
	}

	return bashReaderInstallResult{
		ScriptPath:    scriptPath,
		BashRCPath:    bashRCPath,
		BashRCUpdated: bashRCUpdated,
	}, nil
}

func uninstallBashReaderPlugin(homeDir string, purgeData bool) (bashReaderUninstallResult, error) {
	bashRCPath := getBashRCPath(homeDir)
	pluginDirPath := filepath.Dir(getBashReaderScriptPath(homeDir))
	contextDirPath := getTerminalContextDir(homeDir)

	bashRCUpdated, err := removeBashReaderSourceBlock(bashRCPath)
	if err != nil {
		return bashReaderUninstallResult{}, err
	}

	pluginDirRemoved, err := removeDirIfExists(pluginDirPath)
	if err != nil {
		return bashReaderUninstallResult{}, err
	}

	contextDirRemoved := false
	if purgeData {
		contextDirRemoved, err = removeDirIfExists(contextDirPath)
		if err != nil {
			return bashReaderUninstallResult{}, err
		}
	}

	return bashReaderUninstallResult{
		BashRCPath:        bashRCPath,
		BashRCUpdated:     bashRCUpdated,
		PluginDirPath:     pluginDirPath,
		PluginDirRemoved:  pluginDirRemoved,
		ContextDirPath:    contextDirPath,
		ContextDirRemoved: contextDirRemoved,
	}, nil
}

func ensureBashReaderSourceBlock(bashRCPath string) (bool, error) {
	contentBytes, err := os.ReadFile(bashRCPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return false, fmt.Errorf("failed to read %s: %w", bashRCPath, err)
		}
		contentBytes = []byte{}
	}

	content := string(contentBytes)
	if strings.Contains(content, bashReaderBlockStart) || strings.Contains(content, bashReaderSourceLine) {
		return false, nil
	}

	block := strings.Join([]string{bashReaderBlockStart, bashReaderSourceLine, bashReaderBlockEnd}, "\n") + "\n"

	updatedContent := insertAfterLastTerminalAgentBlock(content, block)

	if err := os.WriteFile(bashRCPath, []byte(updatedContent), 0644); err != nil {
		return false, fmt.Errorf("failed to update %s: %w", bashRCPath, err)
	}

	return true, nil
}

func removeBashReaderSourceBlock(bashRCPath string) (bool, error) {
	contentBytes, err := os.ReadFile(bashRCPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to read %s: %w", bashRCPath, err)
	}

	updatedContent, removed := removeManagedBlock(string(contentBytes), bashReaderBlockStart, bashReaderBlockEnd)
	if !removed {
		return false, nil
	}

	if err := os.WriteFile(bashRCPath, []byte(updatedContent), 0644); err != nil {
		return false, fmt.Errorf("failed to update %s: %w", bashRCPath, err)
	}

	return true, nil
}

func removeManagedBlock(content, blockStart, blockEnd string) (string, bool) {
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))

	insideBlock := false
	removed := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !insideBlock && trimmed == blockStart {
			insideBlock = true
			removed = true
			continue
		}

		if insideBlock {
			if trimmed == blockEnd {
				insideBlock = false
			}
			continue
		}

		result = append(result, line)
	}

	for len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}

	if len(result) == 0 {
		return "", removed
	}

	return strings.Join(result, "\n") + "\n", removed
}

func removeDirIfExists(dirPath string) (bool, error) {
	if _, err := os.Stat(dirPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check %s: %w", dirPath, err)
	}

	if err := os.RemoveAll(dirPath); err != nil {
		return false, fmt.Errorf("failed to remove %s: %w", dirPath, err)
	}

	return true, nil
}

func insertAfterLastTerminalAgentBlock(content, block string) string {
	trimmedContent := content
	if len(trimmedContent) > 0 && !strings.HasSuffix(trimmedContent, "\n") {
		trimmedContent += "\n"
	}

	lines := strings.Split(trimmedContent, "\n")
	lastManagedEndLine := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "# END terminal-agent") {
			lastManagedEndLine = i
		}
	}

	if lastManagedEndLine >= 0 {
		insertLines := strings.Split(strings.TrimSuffix(block, "\n"), "\n")
		newLines := make([]string, 0, len(lines)+len(insertLines))
		newLines = append(newLines, lines[:lastManagedEndLine+1]...)
		newLines = append(newLines, insertLines...)
		newLines = append(newLines, lines[lastManagedEndLine+1:]...)
		result := strings.Join(newLines, "\n")
		if !strings.HasSuffix(result, "\n") {
			result += "\n"
		}
		return result
	}

	if trimmedContent == "" {
		return block
	}

	return trimmedContent + "\n" + block
}

func getBashReaderScriptPath(homeDir string) string {
	return filepath.Join(homeDir, ".config", "terminal-agent", "plugins", bashReaderPluginName, "init.bash")
}

func getBashRCPath(homeDir string) string {
	return filepath.Join(homeDir, ".bashrc")
}

func getTerminalContextDir(homeDir string) string {
	return filepath.Join(homeDir, ".local", "share", "terminal-agent", "terminal-context")
}

const bashReaderInitScript = `# terminal-agent bash-reader plugin
if [[ -z "${BASH_VERSION:-}" ]]; then
  return
fi

if [[ $- != *i* ]]; then
  return
fi

if [[ "${__TA_BASH_READER_INITIALIZED:-0}" -eq 1 ]]; then
  return
fi

__TA_BASH_READER_INITIALIZED=1
__TA_TERMINAL_CONTEXT_DIR="$HOME/.local/share/terminal-agent/terminal-context"
__TA_TERMINAL_SESSIONS_DIR="$__TA_TERMINAL_CONTEXT_DIR/sessions"
__TA_TERMINAL_INDEX_FILE="$__TA_TERMINAL_CONTEXT_DIR/index.log"

mkdir -p "$__TA_TERMINAL_SESSIONS_DIR"

__ta_debug_trap() {
  if [[ "${__TA_HOOK_RUNNING:-0}" -eq 1 ]]; then
    return
  fi

  case "${BASH_COMMAND:-}" in
    __ta_debug_trap*|__ta_prompt_command*)
      return
      ;;
  esac

  __TA_HOOK_RUNNING=1
  __TA_LAST_BASH_COMMAND="${BASH_COMMAND:-}"
  __TA_HOOK_RUNNING=0
}

__TA_PREV_PROMPT_COMMAND="${PROMPT_COMMAND:-}"

__ta_prompt_command() {
  local exit_code=$?
  __TA_HOOK_RUNNING=1

  local cmd
  cmd="$(fc -ln -1 2>/dev/null)"
  if [[ -z "$cmd" ]]; then
    cmd="${__TA_LAST_BASH_COMMAND:-}"
  fi

  local cmd_b64
  cmd_b64="$(printf '%s' "$cmd" | base64 | tr -d '\n')"

  # output_path is intentionally empty to avoid breaking interactive terminal programs.
  printf '%s\t%s\t%s\t%s\n' "$(date +%s)" "$exit_code" "" "$cmd_b64" >> "$__TA_TERMINAL_INDEX_FILE"

  if [[ -n "$__TA_PREV_PROMPT_COMMAND" ]]; then
    eval "$__TA_PREV_PROMPT_COMMAND"
  fi
  __TA_HOOK_RUNNING=0
  return "$exit_code"
}

if [[ ";${PROMPT_COMMAND};" != *";__ta_prompt_command;"* ]]; then
  PROMPT_COMMAND="__ta_prompt_command"
fi
trap '__ta_debug_trap' DEBUG
`
