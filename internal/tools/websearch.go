package tools

import (
	"fmt"
	"os"
	"strings"

	tavilygo "github.com/diverged/tavily-go"
	"github.com/diverged/tavily-go/client"
	"github.com/diverged/tavily-go/models"
	"github.com/laszukdawid/terminal-agent/internal/utils"
)

const (
	NpxTavilyMcpServer = "npx-tavily-mcp-server"
)

const (
	websearchToolName        = "websearch"
	websearchToolDescription = `Websearch tool provides the ability to search the web using DuckDuckGo.
    The input to the tool is a search query. The tool then provides a markdown list of the first few results.`
	duckduckgoAPIURL = "https://api.duckduckgo.com/?q=%s&format=json&pretty=1"
	numResults       = 5 // Number of results to return

	websearchSystemPrompt = `You use the websearch tool to find relevant information based on the user's query.
    The input is provided in English and you provide the search query.
    Your output is a markdown list of the first few results.
    Below are some examples where the query is prefixed with "Q:".

    <examples>
    Q: What is the capital of France?
    - Paris: https://en.wikipedia.org/wiki/Paris
    </examples>
    `
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

	tavily *client.TavilyClient
}

// NewWebsearchTool returns a new WebsearchTool
func NewWebsearchTool() *WebsearchTool {
	logger := *utils.GetLogger()
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

	tavilyKey := os.Getenv("TAVILY_KEY")
	if tavilyKey == "" {
		logger.Warn("Websearch tool requires TAVILY_KEY environment variable to be set")
		return nil
	}
	tavily := tavilygo.NewClient(tavilyKey)

	return &WebsearchTool{
		name:         websearchToolName,
		description:  websearchToolDescription,
		inputSchema:  inputSchema,
		systemPrompt: websearchSystemPrompt,
		tavily:       tavily,
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
	return w.searchTavily(*query)
}

func (w *WebsearchTool) RunSchema(input map[string]any) (string, error) {
	// Extract query from input
	query, ok := input["query"].(string)
	if !ok {
		return "", fmt.Errorf("failed to extract query from tool input")
	}

	return w.searchTavily(query)
}

func (w *WebsearchTool) searchTavily(query string) (string, error) {
	searchReq := models.SearchRequest{
		Query:       query,
		SearchDepth: "basic",
	}

	response, err := tavilygo.Search(w.tavily, searchReq)
	if err != nil {
		return "", fmt.Errorf("failed to make Tavily API request: %w", err)
	}
	if response == nil {
		return "", fmt.Errorf("tavily API response is nil")
	}

	var results strings.Builder
	for _, result := range response.Results {
		results.WriteString(fmt.Sprintf("- [%s](%s)\n", result.Title, result.URL))

	}
	return results.String(), nil
}
