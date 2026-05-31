package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
)

type taskActionFallbackResponse struct {
	Action   string         `json:"action,omitempty"`
	Type     string         `json:"type,omitempty"`
	Tool     string         `json:"tool,omitempty"`
	ToolName string         `json:"tool_name,omitempty"`
	Input    map[string]any `json:"input,omitempty"`
	Answer   string         `json:"answer,omitempty"`
	Thought  string         `json:"thought,omitempty"`
}

const taskActionSystemPromptSuffix = `

For the next response, return exactly one JSON object and no surrounding markdown or prose. Put any brief reasoning inside the "thought" field.`

const taskActionPromptTemplateText = `Decide the next task step.

Return exactly one JSON object. Do not return a schema, Markdown, or code fences.

Valid response shapes:
{"action":"tool","tool":"<tool name>","input":{...},"thought":"brief reason"}
{"action":"final_answer","answer":"<final answer>","thought":"brief reason"}

Rules:
- Use "action", not "type".
- Use "tool", not "tool_name".
- The "input" field must contain the actual tool arguments for the chosen tool.
- Do not return a JSON schema.
- Use "final": true ONLY when raw output is definitely the final user-facing answer: concise, clean, readable, and requiring no interpretation.
- If raw output needs interpretation, filtering, grouping, cleanup, explanation, or validation, do not set "final": true.

Available tools:
{{.Tools}}

Task context:
{{.Context}}

Return valid JSON only.`

const taskActionRetryPromptTemplateText = `Your previous response was invalid.

Previous response:
{{.PreviousResponse}}

Problem:
{{.Problem}}

Return exactly one corrected JSON object and nothing else.

Valid response shapes:
{"action":"tool","tool":"<tool name>","input":{...},"thought":"brief reason"}
{"action":"final_answer","answer":"<final answer>","thought":"brief reason"}

Available tools:
{{.Tools}}

Task context:
{{.Context}}

Return valid JSON only.`

var (
	taskActionPromptTemplate      = template.Must(template.New("task_action_prompt").Parse(taskActionPromptTemplateText))
	taskActionRetryPromptTemplate = template.Must(template.New("task_action_retry_prompt").Parse(taskActionRetryPromptTemplateText))
)

type taskActionPromptTemplateData struct {
	Tools   string
	Context string
}

type taskActionRetryPromptTemplateData struct {
	PreviousResponse string
	Problem          string
	Tools            string
	Context          string
}

func (a *Agent) queryTaskActionFallback(ctx context.Context, params *connector.QueryParams, taskTools map[string]tools.Tool) (connector.LlmResponseWithTools, error) {
	fallbackParams := *params
	fallbackParams.Messages = nil
	fallbackParams.Stream = false
	fallbackParams.OnStream = nil
	fallbackSystemPrompt := buildTaskActionSystemPrompt(params.SysPrompt)
	fallbackParams.SysPrompt = &fallbackSystemPrompt

	var lastRaw string
	var lastErr error
	for attempt := 0; attempt < maxTaskActionFallbackRuns; attempt++ {
		if err := ctx.Err(); err != nil {
			return connector.LlmResponseWithTools{}, err
		}
		prompt := a.buildTaskActionPrompt(params, taskTools, lastRaw, lastErr)
		fallbackParams.UserPrompt = &prompt

		raw, err := a.Connector.Query(ctx, &fallbackParams)
		if err != nil {
			return connector.LlmResponseWithTools{}, err
		}

		parsed, err := parseTaskActionFallbackResponse(raw)
		if err != nil {
			lastRaw = raw
			lastErr = err
			continue
		}

		response, err := buildTaskActionResponse(parsed)
		if err != nil {
			lastRaw = raw
			lastErr = err
			continue
		}

		return response, nil
	}

	return connector.LlmResponseWithTools{}, fmt.Errorf("task fallback produced invalid action after %d attempts: %w", maxTaskActionFallbackRuns, lastErr)
}

func buildTaskActionSystemPrompt(base *string) string {
	if strings.TrimSpace(*base) == "" {
		return strings.TrimSpace(taskActionSystemPromptSuffix)
	}
	return strings.TrimSpace(*base) + taskActionSystemPromptSuffix
}

func buildTaskActionResponse(parsed taskActionFallbackResponse) (connector.LlmResponseWithTools, error) {
	response := connector.LlmResponseWithTools{Response: strings.TrimSpace(parsed.Thought)}
	action := parsed.normalizedAction()
	if action == "final_answer" {
		response.ToolUse = true
		response.ToolName = ToolNameFinalAnswer
		response.ToolInput = map[string]any{"answer": strings.TrimSpace(parsed.Answer)}
		return response, nil
	}
	if action != "tool" {
		return connector.LlmResponseWithTools{}, fmt.Errorf("task fallback response has invalid action %q", action)
	}

	toolName := parsed.normalizedToolName()
	if toolName == "" {
		return connector.LlmResponseWithTools{}, fmt.Errorf("task fallback response is missing tool name")
	}
	if parsed.Input == nil {
		parsed.Input = map[string]any{}
	}

	response.ToolUse = true
	response.ToolName = toolName
	response.ToolInput = parsed.Input
	return response, nil
}

func (a *Agent) buildTaskActionPrompt(params *connector.QueryParams, taskTools map[string]tools.Tool, previousRaw string, previousErr error) string {
	toolsText := formatTaskActionTools(taskTools)
	contextText := formatTaskActionContext(params)
	if previousErr == nil {
		return renderTaskTemplate(taskActionPromptTemplate, taskActionPromptTemplateData{
			Tools:   toolsText,
			Context: contextText,
		})
	}
	return renderTaskTemplate(
		taskActionRetryPromptTemplate,
		taskActionRetryPromptTemplateData{
			PreviousResponse: TruncateString(strings.TrimSpace(previousRaw), 1200),
			Problem:          previousErr.Error(),
			Tools:            toolsText,
			Context:          contextText,
		},
	)
}

func formatTaskActionTools(execTools map[string]tools.Tool) string {
	names := make([]string, 0, len(execTools))
	for name := range execTools {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]string, 0, len(names))
	for _, name := range names {
		tool := execTools[name]
		entry := fmt.Sprintf("- %s: %s", tool.Name(), tool.Description())
		schemaHints := formatTaskActionSchemaHints(tool.InputSchema())
		if schemaHints != "" {
			entry += "\n" + schemaHints
		}
		entries = append(entries, entry)
	}
	return strings.Join(entries, "\n")
}

func formatTaskActionSchemaHints(schema map[string]any) string {
	properties, ok := normalizeSchemaMap(schema["properties"])
	if !ok || len(properties) == 0 {
		return ""
	}

	required := make(map[string]bool)
	for _, name := range schemaStringSlice(schema["required"]) {
		required[name] = true
	}

	fieldNames := make([]string, 0, len(properties))
	for name := range properties {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)

	lines := make([]string, 0, len(fieldNames)+2)
	for _, fieldName := range fieldNames {
		definition, ok := normalizeSchemaDefinition(properties[fieldName])
		if !ok {
			continue
		}
		line := fmt.Sprintf("  - %s (%s, %s)", fieldName, schemaFieldType(definition), requiredLabel(required[fieldName]))
		if description, _ := definition["description"].(string); description != "" {
			line += ": " + description
		}
		if enumValues := schemaStringSlice(definition["enum"]); len(enumValues) > 0 {
			line += fmt.Sprintf(" Allowed values: %s.", strings.Join(enumValues, ", "))
		}
		lines = append(lines, line)
	}

	if requirementHint := formatTaskActionRequirementHints(schema); requirementHint != "" {
		lines = append(lines, "  - requirement: "+requirementHint)
	}

	return strings.Join(lines, "\n")
}

func schemaFieldType(definition map[string]any) string {
	if typeName, _ := definition["type"].(string); typeName != "" {
		return typeName
	}
	return "value"
}

func requiredLabel(required bool) string {
	if required {
		return "required"
	}
	return "optional"
}

func formatTaskActionRequirementHints(schema map[string]any) string {
	branches := schemaAnySlice(schema["oneOf"])
	label := "exactly one of"
	if len(branches) == 0 {
		branches = schemaAnySlice(schema["anyOf"])
		label = "at least one of"
	}
	if len(branches) == 0 {
		return ""
	}

	options := make([]string, 0, len(branches))
	for _, branch := range branches {
		branchSchema, ok := normalizeSchemaDefinition(branch)
		if !ok {
			continue
		}
		requiredFields := schemaStringSlice(branchSchema["required"])
		if len(requiredFields) == 0 {
			continue
		}
		options = append(options, strings.Join(requiredFields, ", "))
	}
	if len(options) == 0 {
		return ""
	}
	return fmt.Sprintf("provide %s: %s", label, strings.Join(options, " or "))
}

func formatTaskActionContext(params *connector.QueryParams) string {
	if params == nil {
		return ""
	}
	parts := make([]string, 0, len(params.Messages)+1)
	for _, msg := range params.Messages {
		role := strings.ToUpper(strings.TrimSpace(msg.Role))
		if role == "" {
			role = "USER"
		}
		parts = append(parts, fmt.Sprintf("%s: %s", role, msg.Content))
	}
	if params.UserPrompt != nil && *params.UserPrompt != "" {
		parts = append(parts, "USER: "+*params.UserPrompt)
	}
	return strings.Join(parts, "\n\n")
}

func parseTaskActionFallbackResponse(raw string) (taskActionFallbackResponse, error) {
	trimmed := strings.TrimSpace(raw)
	candidates := extractJSONObjects(trimmed)
	if len(candidates) == 0 {
		return taskActionFallbackResponse{}, fmt.Errorf("task fallback response did not contain a valid JSON object")
	}

	var lastErr error
	for _, candidate := range candidates {
		var response taskActionFallbackResponse
		if err := json.Unmarshal([]byte(candidate), &response); err != nil {
			lastErr = fmt.Errorf("failed to parse task fallback response as JSON: %w", err)
			continue
		}

		action := response.normalizedAction()
		if action == "tool" || action == "final_answer" {
			return response, nil
		}
		lastErr = fmt.Errorf("task fallback response has invalid action %q", action)
	}

	if lastErr != nil {
		return taskActionFallbackResponse{}, lastErr
	}
	return taskActionFallbackResponse{}, fmt.Errorf("task fallback response did not contain a valid action object")
}

func (r taskActionFallbackResponse) normalizedAction() string {
	action := strings.TrimSpace(r.Action)
	if action == "" {
		action = strings.TrimSpace(r.Type)
	}
	action = strings.ToLower(action)
	switch action {
	case "final", "answer", "complete", "done":
		return "final_answer"
	default:
		if action == "" {
			if strings.TrimSpace(r.Answer) != "" {
				return "final_answer"
			}
			if r.Input != nil || strings.TrimSpace(r.Tool) != "" || strings.TrimSpace(r.ToolName) != "" {
				return "tool"
			}
		}
		return action
	}
}

func (r taskActionFallbackResponse) normalizedToolName() string {
	if toolName := strings.TrimSpace(r.Tool); toolName != "" {
		return toolName
	}
	return strings.TrimSpace(r.ToolName)
}

func extractJSONObjects(raw string) []string {
	if raw == "" {
		return nil
	}

	objects := []string{}
	depth := 0
	start := -1
	inString := false
	escaped := false

	for i, r := range raw {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch r {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch r {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				objects = append(objects, raw[start:i+1])
				start = -1
			}
		}
	}

	return objects
}
