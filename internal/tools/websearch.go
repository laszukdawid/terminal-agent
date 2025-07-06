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
	websearchToolName        = "websearch"
	websearchToolDescription = `Websearch tool provides the ability to search the web using Tavily.
    The input to the tool is a search query. The tool then provides a markdown list of the first few results.`
	numResults = 5 // Number of results to return

	websearchSystemPrompt = `You use the websearch tool to find relevant information based on the user's query.
    The input is provided in English and you provide the search query.
    Your output is a markdown list of the first few results.
    Below are some examples where the query is prefixed with "Q:".

    <examples>
    Q: What is the capital of France?
    - Paris: https://en.wikipedia.org/wiki/Paris
    </examples>
    `
	websearchToolHelp = `WEBSEARCH TOOL HELP
==================

NAME
    websearch - Search the web and retrieve relevant results

DESCRIPTION
    The websearch tool allows searching the internet for up-to-date information.
    It takes a search query as input and returns a markdown-formatted list of relevant search results
    from the web. The tool uses the Tavily search API to perform web searches.

USAGE
    agent tool exec websearch [query]
    
    Examples:
      agent tool exec websearch "latest golang release"
      agent tool exec websearch "weather in Warsaw"
      agent tool exec websearch "how to implement rate limiting in go"

INPUT SCHEMA
    {
      "query": "The search query to use for the web search (string)"
    }

OUTPUT
    The tool returns a markdown-formatted list of search results, each with a title and URL:
    
    - [Result Title 1](https://url1.example.com)
    - [Result Title 2](https://url2.example.com)
    - ...

REQUIREMENTS
    - Requires a Tavily API key set as the TAVILY_KEY environment variable
`
)

// WebsearchTool Tool implements the Tool interface
type WebsearchTool struct {
	name         string
	description  string
	inputSchema  map[string]any
	systemPrompt string
	helpText     string

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
		logger.Debug("Websearch tool requires TAVILY_KEY environment variable to be set")
		return nil
	}
	tavily := tavilygo.NewClient(tavilyKey)

	return &WebsearchTool{
		name:         websearchToolName,
		description:  websearchToolDescription,
		inputSchema:  inputSchema,
		systemPrompt: systemPrompt,
		helpText:     websearchToolHelp,
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

// HelpText returns detailed help information for the websearch tool
func (w *WebsearchTool) HelpText() string {
	return w.helpText
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
