package connector

import (
	"fmt"
	"strings"
)

// ANSI color codes
const (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Italic = "\033[3m"

	// Colors for syntax highlighting
	CodeColor    = "\033[38;5;252m\033[48;5;235m" // Light gray on dark background
	KeywordColor = "\033[38;5;204m"               // Pink for keywords
	StringColor  = "\033[38;5;150m"               // Light green for strings
	CommentColor = "\033[38;5;244m"               // Gray for comments
)

// MarkdownStreamRenderer handles streaming markdown content with proper formatting
type MarkdownStreamRenderer struct {
	totalBuffer   strings.Builder
	pendingBuffer strings.Builder
	inCodeBlock   bool
}

// NewMarkdownStreamRenderer creates a new markdown stream renderer
func NewMarkdownStreamRenderer() (*MarkdownStreamRenderer, error) {
	return &MarkdownStreamRenderer{
		totalBuffer:   strings.Builder{},
		pendingBuffer: strings.Builder{},
		inCodeBlock:   false,
	}, nil
}

// ProcessChunk processes a chunk of text and outputs formatted markdown
func (m *MarkdownStreamRenderer) ProcessChunk(chunk string) {
	// Add to both buffers
	m.totalBuffer.WriteString(chunk)
	m.pendingBuffer.WriteString(chunk)

	// Process the pending buffer for complete markdown elements
	pending := m.pendingBuffer.String()

	// Check for code blocks
	codeBlockCount := strings.Count(m.totalBuffer.String(), "```")
	if codeBlockCount%2 == 1 {
		if !m.inCodeBlock {
			// We just entered a code block
			m.inCodeBlock = true
		}
	} else {
		if m.inCodeBlock {
			// We just exited a code block
			m.inCodeBlock = false
		}
	}

	// For better streaming, process smaller chunks more frequently
	if strings.Contains(pending, "```") {
		// Handle code blocks
		parts := strings.Split(pending, "```")
		for i, part := range parts {
			if i%2 == 0 {
				// Regular text
				m.printFormattedText(part)
			} else {
				// Code block
				fmt.Print(CodeColor + part + Reset)
			}
		}
		m.pendingBuffer.Reset()
	} else if strings.Contains(pending, "`") && strings.Count(pending, "`")%2 == 0 {
		// Handle inline code
		parts := strings.Split(pending, "`")
		for i, part := range parts {
			if i%2 == 0 {
				// Regular text
				m.printFormattedText(part)
			} else {
				// Inline code
				fmt.Print(CodeColor + part + Reset)
			}
		}
		m.pendingBuffer.Reset()
	} else if len(pending) > 50 { // Reduced threshold for more responsive streaming
		// If buffer is getting large without complete elements, flush most of it
		toFlush := pending[:40]
		m.printFormattedText(toFlush)
		m.pendingBuffer.Reset()
		m.pendingBuffer.WriteString(pending[40:])
	}
}

// printFormattedText applies basic formatting to text
func (m *MarkdownStreamRenderer) printFormattedText(text string) {
	// Handle bold text
	if strings.Contains(text, "**") {
		parts := strings.Split(text, "**")
		for i, part := range parts {
			if i%2 == 0 {
				fmt.Print(part)
			} else {
				fmt.Print(Bold + part + Reset)
			}
		}
	} else {
		fmt.Print(text)
	}
}

// Flush ensures any remaining content is rendered
func (m *MarkdownStreamRenderer) Flush() {
	// Print any remaining content
	remaining := m.pendingBuffer.String()
	if remaining != "" {
		m.printFormattedText(remaining)
	}

	// Reset formatting
	fmt.Print(Reset)
}

// SimpleMarkdownStream provides a simpler approach that formats common markdown elements
func SimpleMarkdownStream(chunk string) {
	// This is a simpler alternative that doesn't require buffering
	// It handles basic markdown elements during streaming

	// Handle code blocks
	if strings.Contains(chunk, "```") {
		fmt.Print(chunk)
		return
	}

	// For now, just print the chunk as-is
	// More sophisticated handling can be added later
	fmt.Print(chunk)
}
