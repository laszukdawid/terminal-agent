package tools

import (
	"fmt"
	"strings"
)

func trimmedStringInput(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func integerInput(input map[string]any, key string) (int, bool) {
	switch value := input[key].(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}

func formatStatus(prefix string, parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return prefix + strings.Join(parts, " ")
}

func quotedStatusPart(key string, value string) string {
	return fmt.Sprintf("%s=%q", key, value)
}
