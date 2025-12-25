package memory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/utils"
)

type Memory interface {
	// Add stores a new entry if it doesn't already exist
	Add(content string) error

	// List returns all memory entries
	List() ([]MemoryEntry, error)

	// FormatAsPrompt returns memory formatted for inclusion in a system prompt
	FormatAsPrompt() (string, error)
}

type memory struct {
	path string
}

func NewMemory(path string) Memory {
	return &memory{path: path}
}

type MemoryEntry struct {
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

func (m *memory) Add(content string) error {
	// Check if entry already exists
	entries, err := m.List()
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing entries: %w", err)
	}

	for _, entry := range entries {
		if entry.Content == content {
			// Silently skip - already exists
			return nil
		}
	}

	// Add new entry
	entry := MemoryEntry{
		Content:   content,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if err := utils.WriteToJSONLFile(m.path, entry); err != nil {
		return fmt.Errorf("failed to write memory entry: %w", err)
	}

	return nil
}

func (m *memory) List() ([]MemoryEntry, error) {
	file, err := os.Open(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []MemoryEntry{}, nil
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var entries []MemoryEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry MemoryEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("failed to unmarshal entry: %w", err)
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return entries, nil
}

func (m *memory) FormatAsPrompt() (string, error) {
	entries, err := m.List()
	if err != nil {
		return "", fmt.Errorf("failed to list memory entries: %w", err)
	}

	if len(entries) == 0 {
		return "", nil
	}

	var lines []string
	for _, entry := range entries {
		lines = append(lines, entry.Content)
	}

	return "<memory>\n" + strings.Join(lines, "\n") + "\n</memory>", nil
}
