package utils

import (
	"encoding/json"
	"fmt"
)

type CodeObject struct {
	Code string `json:"code"`
	Lang string `json:"lang"`
}

type ToolQuery struct {
	Tool        string `json:"tool"`
	Instruction string `json:"instruction"`
	Solved      bool   `json:"solved"`
}

// FindCodeObject finds the code object in the input string.
// It returns the code object and an error if the code object is not found.
func FindCodeObject(input *string) (*CodeObject, error) {

	// Extract all potential code blocks
	preCodeObjects, err := ExtractFirstJSONObjects(input)
	if err != nil {
		return nil, fmt.Errorf("no code object: %v", err)
	}

	var codeObject *CodeObject
	// Find the first code block
	for _, preCodeObject := range preCodeObjects {

		// try parsing the code into json
		codeObject = &CodeObject{}
		err := json.Unmarshal([]byte(preCodeObject), codeObject)
		if err == nil && codeObject.Code != "" {
			return codeObject, nil
		}
	}

	return nil, fmt.Errorf("no code object found in the input")
}

func FindToolQuery(input *string) (*ToolQuery, error) {

	// Extract all potential code blocks
	preToolQueries, err := ExtractFirstJSONObjects(input)
	if err != nil {
		return nil, fmt.Errorf("no tool query: %v", err)
	}

	var toolQuery *ToolQuery
	for _, preToolQuery := range preToolQueries {

		toolQuery = &ToolQuery{}
		err := json.Unmarshal([]byte(preToolQuery), toolQuery)
		if err == nil && toolQuery.Tool != "" {
			return toolQuery, nil
		}
	}

	return nil, fmt.Errorf("no tool query found in the input")
}

func ExtractFirstJSONObjects(input *string) ([]string, error) {
	var jsonObjects []string
	var stack []int

	start := -1

	for i, char := range *input {
		if char == '{' {
			if len(stack) == 0 {
				start = i
			}
			stack = append(stack, i)
		} else if char == '}' {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
				if len(stack) == 0 {
					jsonObjects = append(jsonObjects, (*input)[start:i+1])
				}
			}
		}
	}

	if len(jsonObjects) == 0 {
		return nil, fmt.Errorf("no JSON objects found in the input")
	}

	return jsonObjects, nil
}
