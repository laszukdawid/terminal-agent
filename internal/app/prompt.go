package app

import (
	"fmt"

	"github.com/laszukdawid/terminal-agent/internal/memory"
)

func BuildAskPrompt(basePrompt string, includeMemory bool, memoryPath string) (string, error) {
	if !includeMemory {
		return basePrompt, nil
	}

	mClient := memory.NewMemory(memoryPath)
	memoryPrompt, err := mClient.FormatAsPrompt()
	if err != nil {
		return "", fmt.Errorf("failed to format memory: %w", err)
	}

	if memoryPrompt == "" {
		return basePrompt, nil
	}

	return memoryPrompt + "\n\n" + basePrompt, nil
}
