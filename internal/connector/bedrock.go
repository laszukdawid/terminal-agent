package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
	"github.com/laszukdawid/terminal-agent/internal/cloud"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
)

type BedrockModelID string

const (
	BedrockProvider = "bedrock"
	ToolUse         = "tool_use"

	DefaultAwsRegion       = "us-east-1"
	pricingAPIRegion       = "us-east-1"
	bedrockPricingPageURL  = "https://aws.amazon.com/bedrock/pricing/"
	bedrockPriceCacheTTL   = 24 * time.Hour
	bedrockPriceTimeout    = 3 * time.Second
	bedrockPriceMaxResults = 50

	// Supported models https://docs.aws.amazon.com/bedrock/latest/userguide/conversation-inference-supported-models-features.html
	GLM47Flash       BedrockModelID = "zai.glm-4.7-flash"
	ClaudeHaiku      BedrockModelID = "anthropic.claude-3-haiku-20240307-v1:0"
	JambaMini        BedrockModelID = "ai21.jamba-1-5-mini-v1:0"
	MistralSmall2402 BedrockModelID = "mistral.mistral-small-2402-v1:0"
	MistralLarge2402 BedrockModelID = "mistral.mistral-large-2402-v1:0"
)

var (
	ErrBedrockForbidden = fmt.Errorf("bedrock - forbidden")

	bedrockPricingServiceCodes = []string{"AmazonBedrock", "AmazonBedrockFoundationModels", "AmazonBedrockService"}
)

type BedrockConnector struct {
	client     *bedrockruntime.Client
	modelID    BedrockModelID
	modelPrice *BedrockModelPrice
	logger     zap.Logger
}

type BedrockModelPrice struct {
	InputPer1K  float64
	OutputPer1K float64
}

type BedrockSettings struct {
	Profile       string
	Region        string
	CachedPrice   BedrockModelPrice
	HasCachePrice bool
}

type BedrockConfigProvider interface {
	GetBedrockProfile() string
	GetBedrockRegion() string
}

type BedrockPriceConfigProvider interface {
	GetBedrockModelPrice(region string, modelID string) (inputPer1K, outputPer1K float64, lastChecked string, ok bool)
}

type bedrockPriceCacheConfigProvider interface {
	BedrockPriceConfigProvider
	SetBedrockModelPrice(region string, modelID string, inputPer1K, outputPer1K float64, lastChecked string) error
}

func computePriceBedrock(usage *BedrockUsage, modelPrice *BedrockModelPrice) *LLMPrice {
	if modelPrice == nil {
		return nil
	}
	ip := modelPrice.InputPer1K * float64(usage.InputTokens) / 1000
	op := modelPrice.OutputPer1K * float64(usage.OutputTokens) / 1000
	return &LLMPrice{
		InputPrice:  ip,
		OutputPrice: op,
		TotalPrice:  ip + op,
	}
}

func bedrockInferenceConfig(qParams *QueryParams) *types.InferenceConfiguration {
	if qParams.MaxTokens <= 0 {
		return nil
	}

	maxTokens := int32(qParams.MaxTokens)
	return &types.InferenceConfiguration{MaxTokens: &maxTokens}
}

func bedrockUsageFromOutput(usage *types.TokenUsage) *BedrockUsage {
	if usage == nil {
		return nil
	}

	var bedrockUsage BedrockUsage
	if usage.InputTokens != nil {
		bedrockUsage.InputTokens = *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		bedrockUsage.OutputTokens = *usage.OutputTokens
	}
	if usage.TotalTokens != nil {
		bedrockUsage.TotalTokens = *usage.TotalTokens
	}
	return &bedrockUsage
}

func textFromBedrockMessage(modelID BedrockModelID, message types.Message) (string, error) {
	var response string
	for _, content := range message.Content {
		if text, ok := content.(*types.ContentBlockMemberText); ok {
			response += text.Value
		}
	}
	if response == "" {
		return "", fmt.Errorf("model %s returned no text content", modelID)
	}
	return response, nil
}

func bedrockSettingsFromConfig(cfg any, modelID BedrockModelID) BedrockSettings {
	settings := BedrockSettings{}
	provider, ok := cfg.(BedrockConfigProvider)
	if ok {
		settings.Profile = strings.TrimSpace(provider.GetBedrockProfile())
		settings.Region = strings.TrimSpace(provider.GetBedrockRegion())
	}
	priceProvider, ok := cfg.(BedrockPriceConfigProvider)
	if ok {
		input, output, _, found := priceProvider.GetBedrockModelPrice(effectiveBedrockRegion(settings.Region), string(modelID))
		if found {
			settings.CachedPrice = BedrockModelPrice{InputPer1K: input, OutputPer1K: output}
			settings.HasCachePrice = true
		}
	}
	return settings
}

func effectiveBedrockRegion(region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		return DefaultAwsRegion
	}
	return region
}

func ResolveBedrockRegion(ctx context.Context, cfg any) string {
	settings := bedrockSettingsFromConfig(cfg, "")
	sdkConfig, err := cloud.NewAwsConfig(ctx, bedrockAWSConfigOptions(settings)...)
	if err == nil && sdkConfig.Region != "" {
		return sdkConfig.Region
	}
	return effectiveBedrockRegion(settings.Region)
}

func bedrockAWSConfigOptions(settings BedrockSettings) []func(*config.LoadOptions) error {
	var opts []func(*config.LoadOptions) error
	if settings.Profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(settings.Profile))
	}
	if settings.Region != "" {
		opts = append(opts, config.WithRegion(settings.Region))
	}
	return opts
}

func bedrockPricingAWSConfigOptions(settings BedrockSettings) []func(*config.LoadOptions) error {
	var opts []func(*config.LoadOptions) error
	if settings.Profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(settings.Profile))
	}
	opts = append(opts, config.WithRegion(pricingAPIRegion))
	return opts
}

func bedrockModelPriceFromConfig(cfg any, modelID BedrockModelID, region string) *BedrockModelPrice {
	priceProvider, ok := cfg.(BedrockPriceConfigProvider)
	if !ok {
		return nil
	}
	input, output, _, found := priceProvider.GetBedrockModelPrice(effectiveBedrockRegion(region), string(modelID))
	if !found {
		return nil
	}
	return &BedrockModelPrice{InputPer1K: input, OutputPer1K: output}
}

type bedrockPricingClient interface {
	GetProducts(ctx context.Context, params *pricing.GetProductsInput, optFns ...func(*pricing.Options)) (*pricing.GetProductsOutput, error)
}

type awsPriceListProduct struct {
	Product struct {
		Attributes map[string]string `json:"attributes"`
	} `json:"product"`
	Terms struct {
		OnDemand map[string]struct {
			PriceDimensions map[string]struct {
				Description  string            `json:"description"`
				PricePerUnit map[string]string `json:"pricePerUnit"`
				Unit         string            `json:"unit"`
			} `json:"priceDimensions"`
		} `json:"OnDemand"`
	} `json:"terms"`
}

func FetchBedrockModelPrice(ctx context.Context, cfg any, modelID BedrockModelID) (BedrockModelPrice, error) {
	ctx, cancel := context.WithTimeout(ctx, bedrockPriceTimeout)
	defer cancel()

	settings := bedrockSettingsFromConfig(cfg, modelID)
	settings.Region = ResolveBedrockRegion(ctx, cfg)

	sdkConfig, err := cloud.NewAwsConfig(ctx, bedrockPricingAWSConfigOptions(settings)...)
	if err == nil {
		price, priceErr := fetchBedrockModelPrice(ctx, pricing.NewFromConfig(sdkConfig), modelID, settings.Region)
		if priceErr == nil {
			return price, nil
		}
	}

	if err != nil {
		return BedrockModelPrice{}, fmt.Errorf("load AWS pricing configuration: %w", err)
	}
	return BedrockModelPrice{}, fmt.Errorf("pricing not found for Bedrock model %q in region %q", strings.TrimSpace(string(modelID)), settings.Region)
}

func refreshBedrockModelPriceIfNeeded(ctx context.Context, cfg any, modelID BedrockModelID, now time.Time, errWriter io.Writer) {
	ctx, cancel := context.WithTimeout(ctx, bedrockPriceTimeout)
	defer cancel()

	cache, ok := cfg.(bedrockPriceCacheConfigProvider)
	if !ok {
		return
	}
	model := strings.TrimSpace(string(modelID))
	region := ResolveBedrockRegion(ctx, cfg)
	if _, _, lastChecked, ok := cache.GetBedrockModelPrice(region, model); ok && !bedrockPriceCacheExpired(lastChecked, now) {
		return
	}

	price, err := FetchBedrockModelPrice(ctx, cfg, modelID)
	if err != nil {
		fmt.Fprintf(errWriter, "Warning: could not refresh Bedrock pricing for %s: %v\n", model, err)
		fmt.Fprintln(errWriter, "Cost estimates for this Bedrock model will be unavailable until pricing is configured or refreshed successfully.")
		return
	}

	checkedAt := now.UTC().Format(time.RFC3339)
	if err := cache.SetBedrockModelPrice(region, model, price.InputPer1K, price.OutputPer1K, checkedAt); err != nil {
		fmt.Fprintf(errWriter, "Warning: could not save Bedrock pricing for %s: %v\n", model, err)
	}
}

func bedrockPriceCacheExpired(lastChecked string, now time.Time) bool {
	checkedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(lastChecked))
	if err != nil {
		return true
	}
	return now.Sub(checkedAt) > bedrockPriceCacheTTL
}

func fetchBedrockModelPrice(ctx context.Context, client bedrockPricingClient, modelID BedrockModelID, region string) (BedrockModelPrice, error) {
	model := strings.TrimSpace(string(modelID))
	if model == "" {
		return BedrockModelPrice{}, fmt.Errorf("bedrock model ID cannot be empty")
	}

	for _, candidate := range bedrockPricingModelCandidates(model) {
		price, err := fetchBedrockModelPriceCandidate(ctx, client, candidate, region)
		if err == nil {
			return price, nil
		}
	}

	return BedrockModelPrice{}, fmt.Errorf("pricing not found for Bedrock model %q", model)
}

func bedrockPricingModelCandidates(model string) []string {
	seen := map[string]struct{}{}
	var candidates []string
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}

	add(model)
	if strings.HasPrefix(model, "arn:") {
		parts := strings.Split(model, "/")
		add(parts[len(parts)-1])
	}
	for _, prefix := range []string{"us.", "eu.", "apac."} {
		add(strings.TrimPrefix(model, prefix))
	}
	parts := strings.Split(model, ".")
	if len(parts) > 1 {
		withoutProvider := strings.Join(parts[1:], ".")
		add(withoutProvider)
		add(bedrockModelIDToDisplayCandidate(withoutProvider))
		if len(parts) > 2 {
			baseModel := parts[len(parts)-1]
			add(baseModel)
			add(bedrockModelIDToDisplayCandidate(baseModel))
		}
	}
	add(bedrockModelIDToDisplayCandidate(model))
	for _, candidate := range append([]string{}, candidates...) {
		stripped := bedrockStripVersionSuffix(candidate)
		if stripped != candidate {
			add(stripped)
			add(bedrockModelIDToDisplayCandidate(stripped))
		}
	}
	for _, candidate := range append([]string{}, candidates...) {
		add(strings.TrimSuffix(candidate, " IT"))
		add(strings.TrimSuffix(candidate, " Instruct"))
	}
	return candidates
}

func bedrockModelIDToDisplayCandidate(model string) string {
	words := strings.FieldsFunc(model, func(r rune) bool {
		return r == '-' || r == '_' || r == ':' || r == '/' || r == '.'
	})
	for i, word := range words {
		if word == "" {
			continue
		}
		if len(word) <= 3 {
			words[i] = strings.ToUpper(word)
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
	}
	return strings.Join(words, " ")
}

func bedrockStripVersionSuffix(model string) string {
	parts := strings.FieldsFunc(model, func(r rune) bool {
		return r == '-' || r == '_' || r == ':' || r == '.'
	})
	var kept []string
	for _, part := range parts {
		if len(part) >= 4 && isAllDigits(part) {
			break
		}
		lower := strings.ToLower(part)
		if len(lower) >= 2 && lower[0] == 'v' && isAllDigits(lower[1:]) {
			break
		}
		kept = append(kept, part)
	}
	if len(kept) == 0 || len(kept) == len(parts) {
		return model
	}
	return strings.Join(kept, "-")
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func fetchBedrockModelPriceCandidate(ctx context.Context, client bedrockPricingClient, model string, region string) (BedrockModelPrice, error) {
	fields := []string{"modelId", "model", "modelName", "usagetype", "operation"}
	for _, field := range fields {
		price, err := fetchBedrockModelPriceWithFilter(ctx, client, field, model, region)
		if err == nil {
			return price, nil
		}
	}
	return BedrockModelPrice{}, fmt.Errorf("pricing not found for Bedrock model candidate %q", model)
}

func fetchBedrockModelPriceWithFilter(ctx context.Context, client bedrockPricingClient, field string, model string, region string) (BedrockModelPrice, error) {
	var lastErr error
	for _, serviceCode := range bedrockPricingServiceCodes {
		price, err := fetchBedrockModelPriceWithServiceFilter(ctx, client, serviceCode, field, model, region)
		if err == nil {
			return price, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return BedrockModelPrice{}, lastErr
	}
	return BedrockModelPrice{}, fmt.Errorf("pricing dimensions not found")
}

func fetchBedrockModelPriceWithServiceFilter(ctx context.Context, client bedrockPricingClient, serviceCode string, field string, model string, region string) (BedrockModelPrice, error) {
	output, err := client.GetProducts(ctx, &pricing.GetProductsInput{
		ServiceCode:   aws.String(serviceCode),
		FormatVersion: aws.String("aws_v1"),
		MaxResults:    aws.Int32(bedrockPriceMaxResults),
		Filters: []pricingtypes.Filter{
			{Type: pricingtypes.FilterTypeContains, Field: aws.String(field), Value: aws.String(model)},
		},
	})
	if err != nil {
		return BedrockModelPrice{}, err
	}
	for _, rawProduct := range output.PriceList {
		price, ok := parseBedrockPriceListProduct(rawProduct, model, region)
		if ok {
			return price, nil
		}
	}
	return BedrockModelPrice{}, fmt.Errorf("pricing dimensions not found")
}

type bedrockPricingPageHTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func fetchBedrockModelPriceFromPricingPage(ctx context.Context, client bedrockPricingPageHTTPClient, modelID BedrockModelID) (BedrockModelPrice, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bedrockPricingPageURL, nil)
	if err != nil {
		return BedrockModelPrice{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return BedrockModelPrice{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BedrockModelPrice{}, fmt.Errorf("pricing page returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return BedrockModelPrice{}, err
	}
	return parseBedrockPricingPage(string(body), bedrockPricingModelCandidates(string(modelID)))
}

var (
	pricingPageRowPattern  = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	pricingPageCellPattern = regexp.MustCompile(`(?is)<t[dh][^>]*>(.*?)</t[dh]>`)
	pricingPageTagPattern  = regexp.MustCompile(`(?is)<[^>]+>`)
)

func parseBedrockPricingPage(pageHTML string, modelCandidates []string) (BedrockModelPrice, error) {
	for _, row := range pricingPageRowPattern.FindAllStringSubmatch(pageHTML, -1) {
		var cells []string
		for _, cell := range pricingPageCellPattern.FindAllStringSubmatch(row[1], -1) {
			cells = append(cells, normalizePricingPageCell(cell[1]))
		}
		if len(cells) < 3 || !pricingPageCellMatchesAny(cells[0], modelCandidates) {
			continue
		}
		input, inputOK := parsePricingPageUSD(cells[1])
		output, outputOK := parsePricingPageUSD(cells[2])
		if inputOK && outputOK {
			return BedrockModelPrice{InputPer1K: input / 1000, OutputPer1K: output / 1000}, nil
		}
	}
	return BedrockModelPrice{}, fmt.Errorf("pricing page did not contain model pricing")
}

func normalizePricingPageCell(value string) string {
	value = pricingPageTagPattern.ReplaceAllString(value, "")
	value = html.UnescapeString(value)
	return strings.Join(strings.Fields(value), " ")
}

func pricingPageCellMatchesAny(value string, candidates []string) bool {
	normalizedValue := normalizeBedrockPriceMatchText(value)
	for _, candidate := range candidates {
		if normalizedValue == normalizeBedrockPriceMatchText(candidate) {
			return true
		}
	}
	return false
}

func parsePricingPageUSD(value string) (float64, bool) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "$"))
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", ""))
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func parseBedrockPriceListProduct(rawProduct string, model string, region string) (BedrockModelPrice, bool) {
	var product awsPriceListProduct
	if err := json.Unmarshal([]byte(rawProduct), &product); err != nil {
		return BedrockModelPrice{}, false
	}
	if !bedrockProductMatchesModel(product.Product.Attributes, model) {
		return BedrockModelPrice{}, false
	}
	if !bedrockProductMatchesRegion(product.Product.Attributes, region) {
		return BedrockModelPrice{}, false
	}

	var price BedrockModelPrice
	for _, term := range product.Terms.OnDemand {
		for _, dimension := range term.PriceDimensions {
			usd, ok := dimension.PricePerUnit["USD"]
			if !ok {
				continue
			}
			amount, err := strconv.ParseFloat(usd, 64)
			if err != nil {
				continue
			}
			per1K := normalizeBedrockPricePer1K(amount, dimension.Unit, dimension.Description)
			description := strings.ToLower(dimension.Description + " " + dimension.Unit)
			switch {
			case strings.Contains(description, "input"):
				price.InputPer1K = per1K
			case strings.Contains(description, "output"):
				price.OutputPer1K = per1K
			}
		}
	}

	return price, price.InputPer1K > 0 || price.OutputPer1K > 0
}

func bedrockProductMatchesRegion(attributes map[string]string, region string) bool {
	region = strings.TrimSpace(region)
	if region == "" {
		return true
	}
	if regionCode, ok := attributes["regionCode"]; ok {
		return strings.EqualFold(strings.TrimSpace(regionCode), region)
	}
	return true
}

func bedrockProductMatchesModel(attributes map[string]string, model string) bool {
	needle := normalizeBedrockPriceMatchText(model)
	if needle == "" {
		return false
	}
	for _, value := range attributes {
		if strings.Contains(normalizeBedrockPriceMatchText(value), needle) {
			return true
		}
	}
	return false
}

func normalizeBedrockPriceMatchText(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func normalizeBedrockPricePer1K(amount float64, unit string, description string) float64 {
	text := strings.ToLower(unit + " " + description)
	switch {
	case strings.Contains(text, "1m") || strings.Contains(text, "1 million") || strings.Contains(text, "1,000,000"):
		return amount / 1000
	case strings.Contains(text, "1k") || strings.Contains(text, "1 thousand") || strings.Contains(text, "1,000"):
		return amount
	default:
		return amount
	}
}

// buildMessages constructs the messages slice from history + current user prompt
func (bc *BedrockConnector) buildMessages(qParams *QueryParams) []types.Message {
	var messages []types.Message

	// Add conversation history
	for _, msg := range qParams.Messages {
		messages = append(messages, types.Message{
			Role:    types.ConversationRole(msg.Role),
			Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: msg.Content}},
		})
	}

	// Add current user prompt
	if qParams.UserPrompt != nil && *qParams.UserPrompt != "" {
		messages = append(messages, types.Message{
			Role:    "user",
			Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: *qParams.UserPrompt}},
		})
	}

	return messages
}

// convertToolsToBedrock converts tool definitions to Bedrock tool specifications.
func convertToolsToBedrock(tools map[string]tools.Tool) []types.Tool {
	// Define the input schema as a map
	var bedrockTools []types.Tool
	for _, tool := range tools {
		bedrockTool := &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        aws.String(tool.Name()),
				Description: aws.String(tool.Description()),
				InputSchema: &types.ToolInputSchemaMemberJson{
					Value: document.NewLazyDocument(tool.InputSchema()),
				},
			},
		}
		bedrockTools = append(bedrockTools, bedrockTool)
	}
	return bedrockTools
}

func NewBedrockConnector(modelID *BedrockModelID, cfg any) (*BedrockConnector, error) {
	logger := *utils.GetLogger()
	logger.Debug("NewBedrockConnector")

	if modelID == nil || *modelID == "" {
		model := GLM47Flash
		modelID = &model
	}

	settings := bedrockSettingsFromConfig(cfg, *modelID)
	sdkConfig, err := cloud.NewAwsConfig(context.Background(), bedrockAWSConfigOptions(settings)...)

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS configuration for Bedrock: %w", err)
	}
	if sdkConfig.Region == "" {
		sdkConfig.Region = DefaultAwsRegion
	}
	modelPrice := bedrockModelPriceFromConfig(cfg, *modelID, sdkConfig.Region)

	client := bedrockruntime.NewFromConfig(sdkConfig)

	return &BedrockConnector{
		client:     client,
		modelID:    *modelID,
		modelPrice: modelPrice,
		logger:     logger,
	}, nil
}

func (bc *BedrockConnector) queryBedrock(
	ctx context.Context, qParams *QueryParams, toolConfig *types.ToolConfiguration,
) (*bedrockruntime.ConverseOutput, error) {

	// Build messages list from history + current user prompt
	messages := bc.buildMessages(qParams)

	converseInput := &bedrockruntime.ConverseInput{
		ModelId:         (*string)(&bc.modelID),
		InferenceConfig: bedrockInferenceConfig(qParams),
		System: []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: *qParams.SysPrompt},
		},
		Messages: messages,
	}

	if toolConfig != nil {
		converseInput.ToolConfig = toolConfig
	}

	converseOutput, err := bc.client.Converse(ctx, converseInput)
	if err != nil {
		var re *awshttp.ResponseError
		if errors.As(err, &re) {
			bc.logger.Sugar().Errorf("requestID: %s, error: %v", re.ServiceRequestID(), re.Unwrap())
		}

		if re == nil || re.ResponseError == nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}

		if re.ResponseError.HTTPStatusCode() == 403 {
			return nil, ErrBedrockForbidden
		}

		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Only text output is supported
	if converseOutput.Output == nil {
		return nil, fmt.Errorf("model %s returned response is nil", bc.modelID)
	}
	if usage := bedrockUsageFromOutput(converseOutput.Usage); usage != nil {
		price := computePriceBedrock(usage, bc.modelPrice)

		bc.logger.Sugar().Debugw("Usage", "usage", usage, "price", price)
	}

	if _, ok := converseOutput.Output.(*types.ConverseOutputMemberMessage); !ok {
		return nil, fmt.Errorf("model %s returned unknown response type", bc.modelID)
	}

	return converseOutput, nil
}

func (bc *BedrockConnector) queryBedrockStream(
	ctx context.Context, qParams *QueryParams, toolConfig *types.ToolConfiguration,
) (string, error) {
	var mdRenderer *MarkdownStreamRenderer
	if qParams.OnStream == nil {
		renderer, err := NewMarkdownStreamRenderer()
		if err != nil {
			bc.logger.Warn("Failed to create markdown renderer, falling back to plain text", zap.Error(err))
		} else {
			mdRenderer = renderer
		}
	}

	// Build messages list from history + current user prompt
	messages := bc.buildMessages(qParams)

	converseInput := &bedrockruntime.ConverseStreamInput{
		ModelId:         (*string)(&bc.modelID),
		InferenceConfig: bedrockInferenceConfig(qParams),
		System: []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: *qParams.SysPrompt},
		},
		Messages: messages,
	}

	if toolConfig != nil {
		converseInput.ToolConfig = toolConfig
	}

	converseOutput, err := bc.client.ConverseStream(ctx, converseInput)
	if err != nil {
		var re *awshttp.ResponseError
		if errors.As(err, &re) {
			bc.logger.Sugar().Errorf("requestID: %s, error: %v", re.ServiceRequestID(), re.Unwrap())
		}

		if re == nil || re.ResponseError == nil {
			return "", fmt.Errorf("failed to send request: %w", err)
		}

		if re.ResponseError.HTTPStatusCode() == 403 {
			return "", ErrBedrockForbidden
		}

		return "", fmt.Errorf("failed to send request: %w", err)
	}

	var acc string

	stream := converseOutput.GetStream()
	defer stream.Close()
	var events <-chan types.ConverseStreamOutput = stream.Events()

	for _event := range events {
		switch event := _event.(type) {

		// Message start contains info about "role"
		case *types.ConverseStreamOutputMemberMessageStart:
			v := event.Value
			if v.Role != "assistant" {
				bc.logger.Sugar().Debugw("Weird MessageStart", "role", v.Role)
			}

		// Message stop contains info about "stopReason" and "additionalModelResponseFields"
		case *types.ConverseStreamOutputMemberMessageStop:
			v := event.Value
			bc.logger.Sugar().Debugw("MessageStop",
				"stopReason", v.StopReason, "additionalModelResponseFields", v.AdditionalModelResponseFields)

		case *types.ConverseStreamOutputMemberContentBlockStart:
			start := event.Value.Start
			bc.logger.Debug("ContentBlockStart", zap.Any("start", start))

		case *types.ConverseStreamOutputMemberContentBlockStop:
			stop := event.Value
			bc.logger.Debug("ContentBlockStop", zap.Any("stop", stop))

		case *types.ConverseStreamOutputMemberContentBlockDelta:
			chunk, isText := event.Value.Delta.(*types.ContentBlockDeltaMemberText)
			if !isText {
				continue
			}
			if qParams.OnStream != nil {
				if err := qParams.OnStream(chunk.Value); err != nil {
					return "", err
				}
			} else if mdRenderer != nil {
				mdRenderer.ProcessChunk(chunk.Value)
			} else {
				fmt.Print(chunk.Value)
			}
			acc += chunk.Value

		case *types.ConverseStreamOutputMemberMetadata:
			usage := bedrockUsageFromOutput(event.Value.Usage)
			if usage != nil {
				price := computePriceBedrock(usage, bc.modelPrice)
				bc.logger.Sugar().Debugw("Usage", "usage", usage, "price", price)
			}

		default:
			bc.logger.Warn("union is nil or unknown type", zap.Any("event", event))
		}
	}

	// Flush any remaining content
	if mdRenderer != nil {
		mdRenderer.Flush()
	}
	if err := stream.Err(); err != nil {
		return "", fmt.Errorf("failed to read response stream: %w", err)
	}

	return acc, nil
}

func (bc *BedrockConnector) Query(ctx context.Context, qParams *QueryParams) (string, error) {
	bc.logger.Sugar().Debugw("Query", "model", bc.modelID)

	if qParams.Stream {
		return bc.queryBedrockStream(ctx, qParams, nil)
	}

	converseOutput, err := bc.queryBedrock(ctx, qParams, nil)
	if err != nil {
		return "", err
	}

	union := converseOutput.Output
	if union == nil {
		return "", fmt.Errorf("model %s returned response is nil", bc.modelID)
	}

	messageOutput, ok := union.(*types.ConverseOutputMemberMessage)
	if !ok {
		return "", fmt.Errorf("model %s returned unknown response type", bc.modelID)
	}

	return textFromBedrockMessage(bc.modelID, messageOutput.Value)
}

func (bc *BedrockConnector) QueryWithTool(ctx context.Context, qParams *QueryParams, execTools map[string]tools.Tool) (LlmResponseWithTools, error) {
	bc.logger.Sugar().Debugw("Query with tool", "model", bc.modelID)
	response := LlmResponseWithTools{}

	toolConfig := types.ToolConfiguration{
		Tools:      convertToolsToBedrock(execTools),
		ToolChoice: &types.ToolChoiceMemberAuto{},
	}

	//
	converseOutput, err := bc.queryBedrock(ctx, qParams, &toolConfig)
	if err != nil {
		return response, fmt.Errorf("failed to send request: %w", err)
	}

	if converseOutput.StopReason == ToolUse {
		bc.logger.Sugar().Debugw("Tool use", "stopReason", converseOutput.StopReason)
	}

	messageOutput, ok := converseOutput.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return response, fmt.Errorf("model %s returned unknown response type", bc.modelID)
	}

	return parseBedrockToolResponse(bc.modelID, messageOutput.Value)
}

func parseBedrockToolResponse(modelID BedrockModelID, message types.Message) (LlmResponseWithTools, error) {
	response := LlmResponseWithTools{}
	contents := message.Content
	for _, content := range contents {
		if contentBlockMemberText, ok := content.(*types.ContentBlockMemberText); ok {
			response.Response += contentBlockMemberText.Value + "\n"
		}

		if contentBlockMemberText, ok := content.(*types.ContentBlockMemberToolUse); ok {
			contentToolUse := contentBlockMemberText.Value
			if contentToolUse.Name == nil || *contentToolUse.Name == "" {
				return response, fmt.Errorf("model %s returned tool use with empty name", modelID)
			}
			if contentToolUse.Input == nil {
				return response, fmt.Errorf("model %s returned tool use with empty input", modelID)
			}
			response.ToolUse = true
			response.ToolName = *contentToolUse.Name

			// Parse tool's input
			var inputMap map[string]any
			err := contentToolUse.Input.UnmarshalSmithyDocument(&inputMap)
			if err != nil {
				return response, fmt.Errorf("failed to unmarshal tool input: %v", err)
			}
			response.ToolInput = inputMap
		}
	}

	return response, nil
}

func (bc *BedrockConnector) SupportsNativeToolCalling() bool {
	return true
}
