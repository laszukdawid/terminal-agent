package connector

import (
	"testing"

	genai "github.com/google/generative-ai-go/genai"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/stretchr/testify/assert"
)

func TestConvertToolsToGoogle(t *testing.T) {

	execTools := map[string]tools.Tool{
		"unix": tools.NewUnixTool(nil),
	}

	// Call convertToolsToGoogle
	toolSpecs, err := convertToolsToGoogle(execTools)
	assert.NoError(t, err, "Expected no error, got %v", err)

	// Assert the results
	if len(toolSpecs) != 1 {
		t.Fatalf("Expected 1 tool spec, got %d", len(toolSpecs))
	}

	for _, googleTool := range toolSpecs {
		funcDec := googleTool.FunctionDeclarations[0]
		expectedTool := execTools[funcDec.Name]

		assert.Equal(t, funcDec.Name, expectedTool.Name(), "Expected tool name to be %s, got %s", expectedTool.Name(), funcDec.Name)
		assert.Equal(t, funcDec.Description, expectedTool.Description(), "Expected tool description to be %s, got %s", expectedTool.Description(), funcDec.Description)
		// assert.Equal(t, funcDec.Parameters, expectedTool.InputSchema(), "Expected tool input schema to be %v, got %v", expectedTool.InputSchema(), funcDec.Parameters)
	}

}

func TestConvertToGenaiSchema(t *testing.T) {

	// Define test cases
	tests := []struct {
		name     string
		input    map[string]any
		expected genai.Schema
	}{
		{
			name:  "Unix command schema",
			input: tools.NewUnixTool(nil).InputSchema(),
			expected: genai.Schema{
				Type:        genai.TypeObject,
				Description: "",
				Properties: map[string]*genai.Schema{
					"command": {
						Type: genai.TypeString,
						Description: "The Unix command to execute. " +
							"Please provide the command in a single line without any new lines.",
					},
				},
			},
		},
		{
			name: "Empty schema",
			input: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []string{},
			},
			expected: genai.Schema{
				Type:        genai.TypeObject,
				Description: "",
				Properties:  map[string]*genai.Schema{},
				Required:    []string{},
			},
		},
		{
			name: "Missing required",
			input: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type": "string",

						"description": "The Unix command to execute.",
					},
				},
			},
			expected: genai.Schema{
				Type:        genai.TypeObject,
				Description: "",
				Properties: map[string]*genai.Schema{
					"command": {
						Type:        genai.TypeString,
						Description: "The Unix command to execute.",
					},
				},
				Required: []string{},
			},
		},
		{
			name: "Single command property",
			input: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The Unix command to execute.",
					},
				},
				"required": []string{"command"},
			},
			expected: genai.Schema{
				Type:        genai.TypeObject,
				Description: "",
				Properties: map[string]*genai.Schema{
					"command": {
						Type:        genai.TypeString,
						Description: "The Unix command to execute.",
					},
				},
				Required: []string{"command"},
			},
		},
		{
			name: "Multiple properties",
			input: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"foo": map[string]any{
						"type":        "integer",
						"description": "An integer property.",
					},
					"bar": map[string]any{
						"type":        "boolean",
						"description": "A boolean property.",
					},
					"baz": map[string]any{
						"type":        "array",
						"description": "An array property.",
						"items": map[string]any{
							"type":        "string",
							"description": "An item in the array.",
						},
					},
				},
			},
			expected: genai.Schema{
				Type:        genai.TypeObject,
				Description: "",
				Properties: map[string]*genai.Schema{
					"foo": {
						Type:        genai.TypeInteger,
						Description: "An integer property.",
					},
					"bar": {
						Type:        genai.TypeBoolean,
						Description: "A boolean property.",
					},
					"baz": {
						Type:        genai.TypeArray,
						Description: "An array property.",
						Items: &genai.Schema{
							Type:        genai.TypeString,
							Description: "An item in the array.",
						},
					},
				},
				Required: []string{},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Call the function to test
			result, err := convertInputSchemaToGenaiSchema(test.input)
			assert.NoError(t, err, "Expected no error, got %v", err)

			// Check if the result matches the expected output
			assert.Equal(t, test.expected.Type, result.Type, "Expected type to be %v, got %v", test.expected.Type, result.Type)
			assert.Equal(t, test.expected.Description, result.Description, "Expected description to be %v, got %v", test.expected.Description, result.Description)
			assert.ElementsMatch(t, test.expected.Required, result.Required, "Expected required to be %v, got %v", test.expected.Required, result.Required)
			assert.Equal(t, len(test.expected.Properties), len(result.Properties), "Expected %d properties, got %d", len(test.expected.Properties), len(result.Properties))
			for key, expectedProp := range test.expected.Properties {
				actualProp, ok := result.Properties[key]
				if !ok {
					t.Errorf("Expected property '%s' to exist", key)
					continue
				}
				assert.Equal(t, expectedProp.Type, actualProp.Type, "Expected property '%s' type to be %v, got %v", key, expectedProp.Type, actualProp.Type)
				assert.Equal(t, expectedProp.Description, actualProp.Description, "Expected property '%s' description to be %v, got %v", key, expectedProp.Description, actualProp.Description)
				if expectedProp.Items != nil {
					assert.NotNil(t, actualProp.Items, "Expected property '%s' to have items", key)
					assert.Equal(t, expectedProp.Items.Type, actualProp.Items.Type, "Expected property '%s' items type to be %v, got %v", key, expectedProp.Items.Type, actualProp.Items.Type)
					assert.Equal(t, expectedProp.Items.Description, actualProp.Items.Description, "Expected property '%s' items description to be %v, got %v", key, expectedProp.Items.Description, actualProp.Items.Description)
				} else {
					assert.Nil(t, actualProp.Items, "Expected property '%s' to not have items", key)
				}
			}
		})
	}

}

func TestNewGoogleConnector(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test")
	modelID := "gemini-2.0-flash-lite"

	connector := NewGoogleConnector(&modelID)

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

	connector := NewGoogleConnector(&modelID)

	assert.Nil(t, connector, "Expected connector to be nil when GEMINI_API_KEY is not set")
}

func TestNewGoogleConnectorNoModelID(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test")
	var modelID *string

	connector := NewGoogleConnector(modelID)

	if connector == nil {
		t.Fatal("Expected connector to be created, got nil")
	}

	if connector.modelID != "gemini-2.0-flash-lite" {
		t.Errorf("Expected modelID to be gemini-2.0-flash-lite, got %s", connector.modelID)
	}
}
