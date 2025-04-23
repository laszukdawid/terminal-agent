package connector

import (
	"reflect"
	"testing"

	genai "github.com/google/generative-ai-go/genai"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/stretchr/testify/assert"
)

func TestConvertToolsToGoogle(t *testing.T) {
	t.Skip("Skipping test for now")
	// Mock UnixTool
	unixTool := tools.NewUnixTool(nil)

	execTools := map[string]tools.Tool{
		"unix": unixTool,
	}

	// Call convertToolsToGoogle
	toolSpecs := convertToolsToGoogle(execTools)

	// Assert the results
	if len(toolSpecs) != 1 {
		t.Fatalf("Expected 1 tool spec, got %d", len(toolSpecs))
	}

	toolSpec := toolSpecs[0]

	if toolSpec.FunctionDeclarations[0].Name != "unix" {
		t.Errorf("Expected tool name to be 'unix', got '%s'", toolSpec.FunctionDeclarations[0].Name)
	}

	if toolSpec.FunctionDeclarations[0].Description != unixTool.Description() {
		t.Errorf("Expected tool description to be '%s', got '%s'", unixTool.Description(), toolSpec.FunctionDeclarations[0].Description)
	}

	expectedParams := convertToGenaiSchema(unixTool.InputSchema())
	if !reflect.DeepEqual(toolSpec.FunctionDeclarations[0].Parameters, expectedParams) {
		t.Errorf("Expected tool parameters to be '%v', got '%v'", expectedParams, toolSpec.FunctionDeclarations[0].Parameters)
	}
}

func TestConvertToGenaiSchema(t *testing.T) {
	inputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The Unix command to execute.",
			},
		},
		"required": []string{"command"},
	}

	genaiSchema := convertToGenaiSchema(inputSchema)

	if genaiSchema.Type != genai.TypeObject {
		t.Errorf("Expected type to be TypeObject, got %v", genaiSchema.Type)
	}

	if len(genaiSchema.Properties) != 1 {
		t.Errorf("Expected 1 property, got %d", len(genaiSchema.Properties))
	}

	commandProp, ok := genaiSchema.Properties["command"]
	if !ok {
		t.Errorf("Expected property 'command' to exist")
	}

	if commandProp.Type != genai.TypeString {
		t.Errorf("Expected command type to be TypeString, got %v", commandProp.Type)
	}

	if commandProp.Description != "The Unix command to execute." {
		t.Errorf("Expected command description to be 'The Unix command to execute.', got %v", commandProp.Description)
	}

	if len(genaiSchema.Required) != 1 {
		t.Errorf("Expected 1 required field, got %d", len(genaiSchema.Required))
	}

	if genaiSchema.Required[0] != "command" {
		t.Errorf("Expected required field to be 'command', got %v", genaiSchema.Required[0])
	}
}

func TestNewGoogleConnector(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test")
	modelID := "gemini-2.0-flash-lite"
	execTools := map[string]tools.Tool{}

	connector := NewGoogleConnector(&modelID, execTools)

	if connector == nil {
		t.Fatal("Expected connector to be created, got nil")
	}

	if connector.modelID != modelID {
		t.Errorf("Expected modelID to be %s, got %s", modelID, connector.modelID)
	}
}

func TestNewGoogleConnectorNoKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	modelID := "gemini-2.0-flash-lite"
	execTools := map[string]tools.Tool{}

	connector := NewGoogleConnector(&modelID, execTools)

	assert.Nil(t, connector, "Expected connector to be nil when GEMINI_API_KEY is not set")
}

func TestNewGoogleConnectorNoModelID(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test")
	var modelID *string
	execTools := map[string]tools.Tool{}

	connector := NewGoogleConnector(modelID, execTools)

	if connector == nil {
		t.Fatal("Expected connector to be created, got nil")
	}

	if connector.modelID != "gemini-2.0-flash-lite" {
		t.Errorf("Expected modelID to be gemini-2.0-flash-lite, got %s", connector.modelID)
	}
}
