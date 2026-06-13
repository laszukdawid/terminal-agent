package tools

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	defaultProcessToolTimeout  = 12 * time.Hour
	defaultProcessToolMaxBytes = 64 * 1024
)

func processOptionsFromInput(input map[string]any) (ProcessOptions, error) {
	timeout := defaultProcessToolTimeout
	if raw, ok := input["timeout"]; ok {
		parsed, err := parseProcessTimeout(raw)
		if err != nil {
			return ProcessOptions{}, err
		}
		timeout = parsed
	}

	maxBytes := defaultProcessToolMaxBytes
	if raw, ok := input["max_bytes"]; ok {
		parsed, err := parseProcessMaxBytes(raw)
		if err != nil {
			return ProcessOptions{}, err
		}
		maxBytes = parsed
	}

	return ProcessOptions{Timeout: timeout, MaxBytes: maxBytes}, nil
}

func parseProcessTimeout(raw any) (time.Duration, error) {
	value, ok := raw.(string)
	if !ok {
		return 0, fmt.Errorf("timeout must be a duration string")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultProcessToolTimeout, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout %q: %w", value, err)
	}
	if duration < 0 {
		return 0, fmt.Errorf("timeout must be non-negative")
	}
	return duration, nil
}

func parseProcessMaxBytes(raw any) (int, error) {
	var maxBytes int
	switch value := raw.(type) {
	case int:
		maxBytes = value
	case int64:
		if value > int64(math.MaxInt) {
			return 0, fmt.Errorf("max_bytes is too large")
		}
		maxBytes = int(value)
	case float64:
		if value != math.Trunc(value) {
			return 0, fmt.Errorf("max_bytes must be an integer")
		}
		if value > float64(math.MaxInt) {
			return 0, fmt.Errorf("max_bytes is too large")
		}
		maxBytes = int(value)
	default:
		return 0, fmt.Errorf("max_bytes must be an integer")
	}
	if maxBytes < 0 {
		return 0, fmt.Errorf("max_bytes must be non-negative")
	}
	return maxBytes, nil
}
