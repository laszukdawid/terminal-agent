package tools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/utils"
)

const (
	websearchToolName        = "websearch"
	websearchToolDescription = `Websearch tool provides the ability to search the web using DuckDuckGo.
    The input to the tool is a search query. The tool then provides a markdown list of the first few results.`
	duckduckgoAPIURL = "https://api.duckduckgo.com/?q=%s&format=json&pretty=1"
	numResults       = 5 // Number of results to return
)

type DuckDuckGoResult struct {
	Heading  string `json:"Heading"`
	FirstURL string `json:"FirstURL"`
}

type DuckDuckGoResponse struct {
	Results []DuckDuckGoResult `json:"Results"`
}

// WebsearchTool Tool implements the Tool interface
type WebsearchTool struct {
	name         string
	description  string
	inputSchema  map[string]any
	systemPrompt string
}

// NewWebsearchTool returns a new WebsearchTool
func NewWebsearchTool() *WebsearchTool {
	inputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]string{
				"type":        "string",
				"description": "The search query to use for the web search.",
			},
		},
		"required": []string{"query"},
	}

	systemPrompt := `You use the websearch tool to find relevant information based on the user's query.
    The input is provided in English and you provide the search query.
    Your output is a markdown list of the first few results.
    Below are some examples where the query is prefixed with "Q:".

    <examples>
    Q: What is the capital of France?
    - Paris: https://en.wikipedia.org/wiki/Paris
    </examples>
    `

	return &WebsearchTool{
		name:         websearchToolName,
		description:  websearchToolDescription,
		inputSchema:  inputSchema,
		systemPrompt: systemPrompt,
	}
}

func (w *WebsearchTool) Name() string {
	return w.name
}

func (w *WebsearchTool) Description() string {
	return w.description
}

func (w *WebsearchTool) InputSchema() map[string]interface{} {
	return w.inputSchema
}

// Run method of Websearch Tool
func (w *WebsearchTool) Run(query *string) (string, error) {
	return w.searchDuckDuckGo(*query)
}

func (w *WebsearchTool) RunSchema(input map[string]interface{}) (string, error) {
	// Extract query from input
	query, ok := input["query"].(string)
	if !ok {
		return "", fmt.Errorf("failed to extract query from tool input")
	}

	return w.searchDuckDuckGo(query)
}

func (w *WebsearchTool) searchDuckDuckGo(query string) (string, error) {
	sugar := utils.Logger.Sugar()
	escapedQuery := url.QueryEscape(query)
	apiURL := fmt.Sprintf(duckduckgoAPIURL, escapedQuery)
	sugar.Debugf("DuckDuckGo API URL: %s", apiURL)

	resp, err := http.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("failed to make DuckDuckGo API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("DuckDuckGo API request failed with status code: %d", resp.StatusCode)
	}

	var duckduckgoResponse DuckDuckGoResponse
	if err := json.NewDecoder(resp.Body).Decode(&duckduckgoResponse); err != nil {
		return "", fmt.Errorf("failed to decode DuckDuckGo API response: %w", err)
	}
	sugar.Debugf("DuckDuckGo API response: %+v", duckduckgoResponse)
	if len(duckduckgoResponse.Results) == 0 {
		return "No results found.", nil
	}

	var results strings.Builder
	for i, result := range duckduckgoResponse.Results {
		if i >= numResults {
			break
		}
		results.WriteString(fmt.Sprintf("- [%s](%s)\n", result.Heading, result.FirstURL))
	}

	if results.String() == "" {
		return "No results found.", nil
	}

	return results.String(), nil
}
