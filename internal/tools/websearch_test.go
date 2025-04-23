package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWebsearchToolHelpText(t *testing.T) {
	// Create a new WebsearchTool instance
	websearchTool := NewWebsearchTool()

	// Call the HelpText method
	helpText := websearchTool.HelpText()

	// Verify that the help text matches the expected constant
	assert.Equal(t, websearchToolHelp, helpText)

	// Verify that the help text contains expected sections
	assert.Contains(t, helpText, "WEBSEARCH TOOL HELP")
	assert.Contains(t, helpText, "NAME")
	assert.Contains(t, helpText, "DESCRIPTION")
	assert.Contains(t, helpText, "USAGE")
	assert.Contains(t, helpText, "INPUT SCHEMA")
	assert.Contains(t, helpText, "OUTPUT")
	assert.Contains(t, helpText, "REQUIREMENTS")
}
