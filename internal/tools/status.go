package tools

import "strings"

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

func joinStatusParts(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, " ")
}

func joinSearchScope(root string, pattern string) string {
	root = strings.TrimSuffix(root, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	switch {
	case root != "" && pattern != "":
		return root + "/" + pattern
	case root != "":
		return root
	default:
		return pattern
	}
}
