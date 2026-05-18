package agent

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/tools"
)

// This validator intentionally handles only the JSON Schema subset we use in
// tool definitions today. A fuller implementation would likely be better served
// by a dedicated library such as github.com/santhosh-tekuri/jsonschema/v6, but
// we are not adopting that dependency here yet.

func resolveTaskToolCall(toolName string, input map[string]any, available map[string]tools.Tool) (tools.Tool, error) {
	tool, ok := available[toolName]
	if !ok || tool == nil {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}

	if err := validateToolInput(tool.InputSchema(), input); err != nil {
		return nil, fmt.Errorf("invalid tool input: %w", err)
	}

	return tool, nil
}

func validateToolInput(schema map[string]any, input map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	if input == nil {
		input = map[string]any{}
	}

	schemaType, _ := schema["type"].(string)
	if schemaType != "" && schemaType != "object" {
		return nil
	}

	for _, field := range schemaStringSlice(schema["required"]) {
		if _, ok := input[field]; !ok {
			return fmt.Errorf("missing required field %q", field)
		}
	}

	if err := validateCombinators(schema, input); err != nil {
		return err
	}

	properties, ok := normalizeSchemaMap(schema["properties"])
	if !ok {
		return nil
	}

	for key, value := range input {
		propertySchema, ok := properties[key]
		if !ok {
			continue
		}
		if err := validateSchemaValue(key, propertySchema, value); err != nil {
			return err
		}
	}

	return nil
}

func validateCombinators(schema map[string]any, input map[string]any) error {
	if err := validateAnyOf(schemaAnySlice(schema["anyOf"]), input); err != nil {
		return err
	}
	if err := validateOneOf(schemaAnySlice(schema["oneOf"]), input); err != nil {
		return err
	}
	return nil
}

func validateAnyOf(branches []any, input map[string]any) error {
	if len(branches) == 0 {
		return nil
	}

	for _, branch := range branches {
		branchSchema, ok := normalizeSchemaDefinition(branch)
		if !ok {
			continue
		}
		if validateToolInput(branchSchema, input) == nil {
			return nil
		}
	}

	return fmt.Errorf("input must satisfy at least one allowed field combination")
}

func validateOneOf(branches []any, input map[string]any) error {
	if len(branches) == 0 {
		return nil
	}

	matches := 0
	for _, branch := range branches {
		branchSchema, ok := normalizeSchemaDefinition(branch)
		if !ok {
			continue
		}
		if validateToolInput(branchSchema, input) == nil {
			matches++
		}
	}

	if matches == 1 {
		return nil
	}
	if matches == 0 {
		return fmt.Errorf("input must satisfy exactly one allowed field combination")
	}
	return fmt.Errorf("input matches multiple mutually exclusive field combinations")
}

func validateSchemaValue(field string, rawSchema any, value any) error {
	schema, ok := normalizeSchemaDefinition(rawSchema)
	if !ok {
		return nil
	}

	if err := validateEnum(field, schema, value); err != nil {
		return err
	}

	typeName, _ := schema["type"].(string)
	switch typeName {
	case "", "object":
		if typeName == "object" {
			if _, ok := value.(map[string]any); !ok {
				return fmt.Errorf("field %q must be an object", field)
			}
		}
		return nil
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("field %q must be a string", field)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("field %q must be a boolean", field)
		}
	case "integer":
		if !isIntegerValue(value) {
			return fmt.Errorf("field %q must be an integer", field)
		}
	case "number":
		if !isNumberValue(value) {
			return fmt.Errorf("field %q must be a number", field)
		}
	case "array":
		items := reflect.ValueOf(value)
		if items.Kind() != reflect.Slice && items.Kind() != reflect.Array {
			return fmt.Errorf("field %q must be an array", field)
		}
		itemSchema, hasItemSchema := schema["items"]
		if !hasItemSchema {
			return nil
		}
		for i := 0; i < items.Len(); i++ {
			if err := validateSchemaValue(fmt.Sprintf("%s[%d]", field, i), itemSchema, items.Index(i).Interface()); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateEnum(field string, schema map[string]any, value any) error {
	allowedValues := schemaAnySlice(schema["enum"])
	if len(allowedValues) == 0 {
		return nil
	}

	for _, allowed := range allowedValues {
		if reflect.DeepEqual(allowed, value) {
			return nil
		}
	}

	allowed := make([]string, 0, len(allowedValues))
	for _, candidate := range allowedValues {
		allowed = append(allowed, fmt.Sprint(candidate))
	}

	return fmt.Errorf("field %q must be one of [%s]", field, strings.Join(allowed, ", "))
}

func normalizeSchemaMap(raw any) (map[string]any, bool) {
	switch typed := raw.(type) {
	case map[string]any:
		return typed, true
	case map[string]string:
		normalized := make(map[string]any, len(typed))
		for key, value := range typed {
			normalized[key] = value
		}
		return normalized, true
	default:
		return nil, false
	}
}

func normalizeSchemaDefinition(raw any) (map[string]any, bool) {
	return normalizeSchemaMap(raw)
}

func schemaStringSlice(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return typed
	case []any:
		values := make([]string, 0, len(typed))
		for _, value := range typed {
			stringValue, ok := value.(string)
			if !ok {
				continue
			}
			values = append(values, stringValue)
		}
		return values
	default:
		return nil
	}
}

func schemaAnySlice(raw any) []any {
	switch typed := raw.(type) {
	case []any:
		return typed
	case []string:
		values := make([]any, 0, len(typed))
		for _, value := range typed {
			values = append(values, value)
		}
		return values
	default:
		return nil
	}
}

func isIntegerValue(value any) bool {
	switch typed := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return typed == float32(int64(typed))
	case float64:
		return typed == float64(int64(typed))
	default:
		return false
	}
}

func isNumberValue(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	default:
		return false
	}
}
