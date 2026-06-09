package connector

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

func TestBedrockInferenceConfig(t *testing.T) {
	tests := []struct {
		name      string
		maxTokens int
		wantNil   bool
	}{
		{name: "unset", maxTokens: 0, wantNil: true},
		{name: "negative", maxTokens: -1, wantNil: true},
		{name: "set", maxTokens: 123, wantNil: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := bedrockInferenceConfig(&QueryParams{MaxTokens: tt.maxTokens})
			if tt.wantNil {
				if config != nil {
					t.Fatalf("config = %v, want nil", config)
				}
				return
			}

			if config == nil || config.MaxTokens == nil || *config.MaxTokens != int32(tt.maxTokens) {
				t.Fatalf("MaxTokens = %v, want %d", config, tt.maxTokens)
			}
		})
	}
}

func TestBedrockSettingsFromConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  any
		want BedrockSettings
	}{
		{
			name: "nil config",
			cfg:  nil,
			want: BedrockSettings{},
		},
		{
			name: "unrelated config",
			cfg:  struct{}{},
			want: BedrockSettings{},
		},
		{
			name: "bedrock config",
			cfg: bedrockSettingsTestConfig{
				profile: " dev ",
				region:  " us-west-2 ",
			},
			want: BedrockSettings{Profile: "dev", Region: "us-west-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bedrockSettingsFromConfig(tt.cfg)
			if got != tt.want {
				t.Fatalf("settings = %+v, want %+v", got, tt.want)
			}
		})
	}
}

type bedrockSettingsTestConfig struct {
	profile string
	region  string
}

func (c bedrockSettingsTestConfig) GetBedrockProfile() string { return c.profile }
func (c bedrockSettingsTestConfig) GetBedrockRegion() string  { return c.region }

func TestBedrockUsageFromOutput(t *testing.T) {
	tests := []struct {
		name  string
		usage *types.TokenUsage
		want  *BedrockUsage
	}{
		{name: "nil", usage: nil, want: nil},
		{
			name: "partial",
			usage: &types.TokenUsage{
				InputTokens: aws.Int32(3),
			},
			want: &BedrockUsage{InputTokens: 3},
		},
		{
			name: "complete",
			usage: &types.TokenUsage{
				InputTokens:  aws.Int32(3),
				OutputTokens: aws.Int32(5),
				TotalTokens:  aws.Int32(8),
			},
			want: &BedrockUsage{InputTokens: 3, OutputTokens: 5, TotalTokens: 8},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bedrockUsageFromOutput(tt.usage)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("usage = %v, want nil", got)
				}
				return
			}

			if got == nil || *got != *tt.want {
				t.Fatalf("usage = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTextFromBedrockMessage(t *testing.T) {
	tests := []struct {
		name    string
		message types.Message
		want    string
		wantErr string
	}{
		{
			name: "text blocks",
			message: types.Message{Content: []types.ContentBlock{
				&types.ContentBlockMemberText{Value: "hello"},
				&types.ContentBlockMemberText{Value: " world"},
			}},
			want: "hello world",
		},
		{
			name:    "no text",
			message: types.Message{Content: []types.ContentBlock{}},
			wantErr: "returned no text content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := textFromBedrockMessage(ClaudeHaiku, tt.message)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("text = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseBedrockToolResponse(t *testing.T) {
	tests := []struct {
		name    string
		message types.Message
		want    LlmResponseWithTools
		wantErr string
	}{
		{
			name: "missing tool name",
			message: types.Message{Content: []types.ContentBlock{
				&types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
					Input: document.NewLazyDocument(map[string]any{"query": "bedrock"}),
				}},
			}},
			wantErr: "empty name",
		},
		{
			name: "missing tool input",
			message: types.Message{Content: []types.ContentBlock{
				&types.ContentBlockMemberToolUse{Value: types.ToolUseBlock{
					Name: aws.String("search"),
				}},
			}},
			wantErr: "empty input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBedrockToolResponse(ClaudeHaiku, tt.message)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Response != tt.want.Response || got.ToolUse != tt.want.ToolUse || got.ToolName != tt.want.ToolName {
				t.Fatalf("response = %+v, want %+v", got, tt.want)
			}
		})
	}
}
