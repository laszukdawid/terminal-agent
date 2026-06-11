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
				prices: map[string]map[string]bedrockSettingsTestPrice{
					"us-west-2": {
						string(ClaudeHaiku): {input: 0.1, output: 0.2, lastChecked: "2026-06-09T00:00:00Z", ok: true},
					},
				},
			},
			want: BedrockSettings{
				Profile:       "dev",
				Region:        "us-west-2",
				CachedPrice:   BedrockModelPrice{InputPer1K: 0.1, OutputPer1K: 0.2},
				HasCachePrice: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bedrockSettingsFromConfig(tt.cfg, ClaudeHaiku)
			if got != tt.want {
				t.Fatalf("settings = %+v, want %+v", got, tt.want)
			}
		})
	}
}

type bedrockSettingsTestConfig struct {
	profile string
	region  string
	prices  map[string]map[string]bedrockSettingsTestPrice
}

type bedrockSettingsTestPrice struct {
	input       float64
	output      float64
	lastChecked string
	ok          bool
}

func (c bedrockSettingsTestConfig) GetBedrockProfile() string { return c.profile }
func (c bedrockSettingsTestConfig) GetBedrockRegion() string  { return c.region }
func (c bedrockSettingsTestConfig) GetBedrockModelPrice(region string, modelID string) (float64, float64, string, bool) {
	prices, ok := c.prices[region]
	if !ok {
		return 0, 0, "", false
	}
	price, ok := prices[modelID]
	if !ok {
		return 0, 0, "", false
	}
	return price.input, price.output, price.lastChecked, price.ok
}

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

func TestParseBedrockPriceListProduct(t *testing.T) {
	rawProduct := `{
		"product": {
			"attributes": {
				"modelId": "anthropic.claude-3-haiku-20240307-v1:0"
			}
		},
		"terms": {
			"OnDemand": {
				"term": {
					"priceDimensions": {
						"input": {
							"description": "$0.25 per 1M input tokens",
							"unit": "1M Tokens",
							"pricePerUnit": {"USD": "0.2500000000"}
						},
						"output": {
							"description": "$1.25 per 1M output tokens",
							"unit": "1M Tokens",
							"pricePerUnit": {"USD": "1.2500000000"}
						}
					}
				}
			}
		}
	}`

	price, ok := parseBedrockPriceListProduct(rawProduct, string(ClaudeHaiku), "us-east-1")
	if !ok {
		t.Fatal("expected price to parse")
	}
	if price.InputPer1K != 0.00025 || price.OutputPer1K != 0.00125 {
		t.Fatalf("price = %+v, want input 0.00025 output 0.00125", price)
	}
}

func TestBedrockPricingModelCandidatesIncludeDisplayName(t *testing.T) {
	candidates := bedrockPricingModelCandidates("google.gemma-3-4b-it")
	wants := []string{"Gemma 3 4B IT", "Gemma 3 4B"}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		seen[candidate] = true
	}
	for _, want := range wants {
		if !seen[want] {
			t.Fatalf("candidates = %v, want %q", candidates, want)
		}
	}
}

func TestBedrockPricingModelCandidatesStripsVersionSuffix(t *testing.T) {
	tests := []struct {
		name  string
		model string
		wants []string
	}{
		{
			name:  "anthropic model with date and version",
			model: "anthropic.claude-sonnet-4-20250514-v1:0",
			wants: []string{"claude-sonnet-4", "Claude Sonnet 4"},
		},
		{
			name:  "region-prefixed model",
			model: "us.anthropic.claude-sonnet-4-20250514-v1:0",
			wants: []string{"claude-sonnet-4-20250514-v1:0", "claude-sonnet-4", "Claude Sonnet 4"},
		},
		{
			name:  "mistral with short date",
			model: "mistral.mistral-small-2402-v1:0",
			wants: []string{"mistral-small", "Mistral Small"},
		},
		{
			name:  "model without version suffix",
			model: "google.gemma-3-4b-it",
			wants: []string{"gemma-3-4b-it", "Gemma 3 4B IT"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidates := bedrockPricingModelCandidates(tt.model)
			seen := map[string]bool{}
			for _, c := range candidates {
				seen[c] = true
			}
			for _, want := range tt.wants {
				if !seen[want] {
					t.Errorf("candidates = %v, missing %q", candidates, want)
				}
			}
		})
	}
}

func TestBedrockStripVersionSuffix(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "date suffix", model: "claude-3-haiku-20240307-v1:0", want: "claude-3-haiku"},
		{name: "short date", model: "mistral-small-2402-v1:0", want: "mistral-small"},
		{name: "version only", model: "jamba-1-5-mini-v1:0", want: "jamba-1-5-mini"},
		{name: "no version", model: "gemma-3-4b-it", want: "gemma-3-4b-it"},
		{name: "single word", model: "claude", want: "claude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bedrockStripVersionSuffix(tt.model)
			if got != tt.want {
				t.Fatalf("strip(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestBedrockPricingServiceCodes(t *testing.T) {
	wants := map[string]bool{
		"AmazonBedrock":                 false,
		"AmazonBedrockFoundationModels": false,
		"AmazonBedrockService":          false,
	}
	for _, serviceCode := range bedrockPricingServiceCodes {
		if _, ok := wants[serviceCode]; ok {
			wants[serviceCode] = true
		}
	}
	for serviceCode, found := range wants {
		if !found {
			t.Fatalf("service codes = %v, want %s", bedrockPricingServiceCodes, serviceCode)
		}
	}
}

func TestParseBedrockPricingPage(t *testing.T) {
	page := `<table><tr><td><b>Google models</b></td><td><b>Price per 1M input tokens</b></td><td><b>Price per 1M output tokens</b></td></tr>
<tr><td>Gemma 3 4B</td><td>$ 0.04</td><td>$ 0.08</td></tr></table>`

	price, err := parseBedrockPricingPage(page, bedrockPricingModelCandidates("google.gemma-3-4b-it"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price.InputPer1K != 0.00004 || price.OutputPer1K != 0.00008 {
		t.Fatalf("price = %+v, want input 0.00004 output 0.00008", price)
	}
}

func TestBedrockProductMatchesModelUsesNormalizedText(t *testing.T) {
	attributes := map[string]string{
		"modelName": "GLM 4.7 Flash",
	}
	if !bedrockProductMatchesModel(attributes, "glm-4.7-flash") {
		t.Fatal("expected normalized model match")
	}
}

func TestNormalizeBedrockPricePer1K(t *testing.T) {
	tests := []struct {
		name        string
		amount      float64
		unit        string
		description string
		want        float64
	}{
		{name: "per million", amount: 2.5, unit: "1M Tokens", want: 0.0025},
		{name: "per thousand", amount: 0.0025, unit: "1K Tokens", want: 0.0025},
		{name: "unknown", amount: 0.0025, unit: "Tokens", want: 0.0025},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeBedrockPricePer1K(tt.amount, tt.unit, tt.description)
			if got != tt.want {
				t.Fatalf("price = %v, want %v", got, tt.want)
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
