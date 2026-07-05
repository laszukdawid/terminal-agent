package agent

import (
	"fmt"

	"github.com/laszukdawid/terminal-agent/internal/agent/tasktools"
	"github.com/laszukdawid/terminal-agent/internal/taskmemory"
	"github.com/laszukdawid/terminal-agent/internal/tools"
)

const ToolNameMemorySearch = "memory_search"

func NewAskUserTool(interaction TaskInteraction) tools.Tool {
	return tasktools.NewAskUser(UserClarificationToolName, func(question string) (string, error) {
		if interaction == nil {
			return "", ErrTaskInteractionRequired
		}
		userInput, err := interaction.Clarify(TaskClarificationRequest{Question: question})
		if err != nil {
			return "", fmt.Errorf("user input error: %w", err)
		}
		return userInput, nil
	})
}

func NewFinalAnswerTool() tools.Tool {
	return tasktools.NewFinalAnswer(ToolNameFinalAnswer)
}

func NewChangeDirectoryTool() tools.Tool {
	return tasktools.NewChangeDirectory(ToolNameChangeDirectory)
}

func NewMemorySearchTool(store taskmemory.Store, env taskmemory.Environment) tools.Tool {
	return tasktools.NewMemorySearch(ToolNameMemorySearch, store, env)
}
