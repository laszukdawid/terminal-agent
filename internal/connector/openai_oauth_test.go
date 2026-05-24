package connector

import (
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/openai/openai-go/responses"
)

type stubTool struct{}

func (stubTool) RunSchema(input map[string]any) (string, error) { return "", nil }
func (stubTool) Run(input *string) (string, error)              { return "", nil }
func (stubTool) Name() string                                   { return "search_code" }
func (stubTool) Description() string                            { return "Search code" }
func (stubTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
		},
	}
}
func (stubTool) HelpText() string { return "" }

func TestBuildResponsesParamsIncludesHistoryAndPrompt(t *testing.T) {
	sysPrompt := "You are helpful"
	userPrompt := "latest question"
	params := buildResponsesParams("gpt-4o-mini", &QueryParams{
		SysPrompt:  &sysPrompt,
		UserPrompt: &userPrompt,
		Messages: []Message{
			{Role: "user", Content: "old user"},
			{Role: "assistant", Content: "old answer"},
		},
		MaxTokens: 321,
	})

	if params.Instructions.Value != sysPrompt {
		t.Fatalf("instructions = %q, want %q", params.Instructions.Value, sysPrompt)
	}
	if params.Model != "gpt-4o-mini" {
		t.Fatalf("model = %q, want %q", params.Model, "gpt-4o-mini")
	}
	if params.MaxOutputTokens.Value != 321 {
		t.Fatalf("max output tokens = %d, want %d", params.MaxOutputTokens.Value, 321)
	}
	if len(params.Input.OfInputItemList) != 3 {
		t.Fatalf("input items = %d, want 3", len(params.Input.OfInputItemList))
	}
	if params.Input.OfInputItemList[0].OfMessage == nil || params.Input.OfInputItemList[0].OfMessage.Role != responses.EasyInputMessageRoleUser {
		t.Fatal("expected first input item to be a user message")
	}
	if params.Input.OfInputItemList[1].OfMessage == nil || params.Input.OfInputItemList[1].OfMessage.Role != responses.EasyInputMessageRoleAssistant {
		t.Fatal("expected second input item to be an assistant message")
	}
}

func TestConvertToolsToResponses(t *testing.T) {
	toolSpecs := convertToolsToResponses(map[string]tools.Tool{
		"search_code": stubTool{},
	})

	if len(toolSpecs) != 1 {
		t.Fatalf("tool specs = %d, want 1", len(toolSpecs))
	}
	if toolSpecs[0].OfFunction == nil {
		t.Fatal("expected function tool spec")
	}
	if toolSpecs[0].OfFunction.Name != "search_code" {
		t.Fatalf("tool name = %q, want %q", toolSpecs[0].OfFunction.Name, "search_code")
	}
}
