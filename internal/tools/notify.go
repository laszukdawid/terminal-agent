package tools

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	notifyCommand              = "notify"
	defaultNotifyTitle          = "Terminal Agent"
	notifySentMessage           = "Notification sent."
	notifyCommandMissingMessage = "notify command not found on PATH; install a compatible notify utility and ensure it is available on PATH"

	notifyUrgencyLow      = "low"
	notifyUrgencyNormal   = "normal"
	notifyUrgencyCritical = "critical"

	notifyUserToolDescription = "Send a user-facing notification through the external notify command."
	notifyUserToolHelp        = `NOTIFY USER TOOL HELP
=====================

NAME
    notify_user - Send a notification through the external notify command

DESCRIPTION
    The notify_user tool delegates delivery to a compatible notify executable on PATH.
    It does not implement OS-specific notification backends.

V1 COMMAND CONTRACT
    notify --title "Terminal Agent" --message "CPU usage crossed 50%"

    --message is required.
    --title is optional and defaults to "Terminal Agent".
    --urgency is optional and must be low, normal, or critical.
`
)

type notifyExecutor interface {
	LookPath(file string) (string, error)
	Run(name string, args ...string) (string, error)
}

type commandNotifyExecutor struct{}

func (commandNotifyExecutor) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (commandNotifyExecutor) Run(name string, args ...string) (string, error) {
	output, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

// NotifyUserTool sends user-facing notifications through an external command.
type NotifyUserTool struct {
	executor notifyExecutor
}

func NewNotifyUserTool() *NotifyUserTool {
	return newNotifyUserTool(commandNotifyExecutor{})
}

func newNotifyUserTool(executor notifyExecutor) *NotifyUserTool {
	return &NotifyUserTool{executor: executor}
}

func (t *NotifyUserTool) Name() string {
	return ToolNameNotifyUser
}

func (t *NotifyUserTool) PermissionCategory() PermissionCategory {
	return PermissionExecute
}

func (t *NotifyUserTool) Description() string {
	return notifyUserToolDescription
}

func (t *NotifyUserTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]string{
				"type":        "string",
				"description": "Optional notification title. Defaults to Terminal Agent.",
			},
			"message": map[string]string{
				"type":        "string",
				"description": "Notification message.",
			},
			"urgency": map[string]any{
				"type":        "string",
				"enum":        []string{notifyUrgencyLow, notifyUrgencyNormal, notifyUrgencyCritical},
				"description": "Optional notification urgency.",
			},
		},
		"required": []string{"message"},
	}
}

func (t *NotifyUserTool) HelpText() string {
	return notifyUserToolHelp
}

func (t *NotifyUserTool) Run(message *string) (string, error) {
	if message == nil {
		return "", fmt.Errorf("notification message is required")
	}
	return t.RunSchema(map[string]any{"message": *message})
}

func (t *NotifyUserTool) RunSchema(input map[string]any) (string, error) {
	message, err := requiredNotificationField(input, "message")
	if err != nil {
		return "", err
	}

	title := defaultNotifyTitle
	if rawTitle, ok := input["title"]; ok {
		title, err = requiredNotificationString(rawTitle, "title")
		if err != nil {
			return "", err
		}
	}

	args := []string{"--title", title, "--message", message}
	if rawUrgency, ok := input["urgency"]; ok {
		urgency, err := requiredNotificationString(rawUrgency, "urgency")
		if err != nil {
			return "", err
		}
		if !isNotifyUrgency(urgency) {
			return "", fmt.Errorf("notification urgency must be one of %s, %s, or %s", notifyUrgencyLow, notifyUrgencyNormal, notifyUrgencyCritical)
		}
		args = append(args, "--urgency", urgency)
	}

	if _, err := t.executor.LookPath(notifyCommand); err != nil {
		return "", fmt.Errorf("%s: %w", notifyCommandMissingMessage, err)
	}

	output, err := t.executor.Run(notifyCommand, args...)
	if err != nil {
		return "", fmt.Errorf("failed to send notification: %w", err)
	}
	if output == "" {
		return notifySentMessage, nil
	}
	return output, nil
}

func requiredNotificationField(input map[string]any, name string) (string, error) {
	value, ok := input[name]
	if !ok {
		return "", fmt.Errorf("notification %s is required", name)
	}
	return requiredNotificationString(value, name)
}

func requiredNotificationString(value any, name string) (string, error) {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("notification %s must be a non-empty string", name)
	}
	return text, nil
}

func isNotifyUrgency(urgency string) bool {
	return urgency == notifyUrgencyLow || urgency == notifyUrgencyNormal || urgency == notifyUrgencyCritical
}
