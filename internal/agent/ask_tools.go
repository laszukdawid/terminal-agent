package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
)

// maxAskWebSearchRounds bounds how many websearch calls a single ask run may
// make. Web search is a narrow, read-only capability; a few rounds is enough to
// gather context and answer without turning ask into a full agentic loop.
const maxAskWebSearchRounds = 3

// AskStep is a single observable step of the ask web-search loop. It lets the
// caller (e.g. session logging) record tool activity without depending on the
// task package's TaskStep plumbing, keeping ask decoupled from task internals.
type AskStep struct {
	Iteration  int
	ToolName   string
	ToolInput  map[string]any
	ToolResult string
	Err        error
}

// AskOptions configures an ask run that may use the websearch tool.
type AskOptions struct {
	Query string
	// UseWebSearch is the user's intent for this run. It is necessary but not
	// sufficient: the loop also requires the capability to be present.
	UseWebSearch bool
	// Stream reports whether the caller wants streamed output.
	Stream bool
	// OnStream is the optional sink for streamed output chunks.
	OnStream func(string) error
	// OnStep is the optional sink for web-search tool steps.
	OnStep func(AskStep)
}

// QuestionWithWebSearch answers a question, optionally using the websearch tool
// in a bounded, read-only loop.
//
// The web-search loop runs only when the user enabled it (opts.UseWebSearch)
// AND the capability is actually present: the connector supports native tool
// calling and the websearch tool is available (TAVILY_KEY set). Otherwise it
// falls back to a plain single-shot answer that streams normally.
//
// Tool results are threaded back as text in the user prompt because connectors
// do not carry tool history through QueryParams.Messages, and QueryWithTool does
// not honor streaming. The loop therefore runs non-streaming and emits the final
// answer once when streaming was requested.
func (a *Agent) QuestionWithWebSearch(ctx context.Context, opts AskOptions) (string, error) {
	if strings.TrimSpace(opts.Query) == "" {
		return "", ErrEmptyQuery
	}

	tool, toolConn, ok := a.webSearchCapability()
	if !opts.UseWebSearch || !ok {
		// Either the user disabled web search (fast path) or it is unavailable.
		return a.queryAskPlain(ctx, *a.systemPromptAsk, opts)
	}

	sysPrompt := *a.systemPromptAsk + AskWebSearchAddendum
	searched := make(map[string]struct{})
	var transcript []string

	for round := 0; round < maxAskWebSearchRounds; round++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		userPrompt := buildAskWebSearchPrompt(opts.Query, transcript, true)
		qParams := connector.QueryParams{
			UserPrompt: &userPrompt,
			SysPrompt:  &sysPrompt,
			MaxTokens:  a.maxTokens,
			Device:     a.device,
		}

		resp, err := toolConn.QueryWithTool(ctx, &qParams, map[string]tools.Tool{tool.Name(): tool})
		if err != nil {
			return "", err
		}
		if !resp.ToolUse || resp.ToolName == "" {
			// Model produced a direct answer.
			return a.emitFinalAnswer(strings.TrimSpace(resp.Response), opts)
		}
		if resp.ToolName != tool.Name() {
			// Only websearch is offered on the ask path; never execute another
			// (possibly hallucinated) tool. Stop and answer from what we have.
			break
		}

		query := webSearchQuery(resp.ToolInput)
		if query == "" {
			// Malformed/empty query: do not spend an API call on it.
			break
		}
		if _, repeated := searched[query]; repeated {
			// Model asked for a search it already ran; stop looping and answer.
			break
		}
		searched[query] = struct{}{}

		if err := ctx.Err(); err != nil {
			return "", err
		}
		result, runErr := tool.RunSchema(resp.ToolInput)
		if opts.OnStep != nil {
			opts.OnStep(AskStep{
				Iteration:  round + 1,
				ToolName:   resp.ToolName,
				ToolInput:  resp.ToolInput,
				ToolResult: result,
				Err:        runErr,
			})
		}
		// A failed search is recorded and the model is allowed to answer anyway,
		// mirroring how the task loop records failures rather than aborting.
		transcript = append(transcript, formatAskSearchEntry(query, result, runErr))
	}

	// Loop closed (direct answer not produced, rounds exhausted, or no further
	// search to run): make one final answer turn from the gathered context. This
	// turn has no tools and uses the base ask prompt, so the model answers from
	// the results rather than being told it can keep searching. It streams when
	// requested.
	finalOpts := opts
	finalOpts.Query = buildAskWebSearchPrompt(opts.Query, transcript, false)
	return a.queryAskPlain(ctx, *a.systemPromptAsk, finalOpts)
}

// webSearchCapability returns the websearch tool and a tool-calling connector
// when web search can actually run on the ask path: the tool is available
// (TAVILY_KEY set, hence present in a.Tools), it is read-category (ask never
// prompts for confirmation), and the connector supports native tool calling.
func (a *Agent) webSearchCapability() (tools.Tool, connector.ToolCallingConnector, bool) {
	tool, ok := a.Tools[tools.ToolNameWebsearch]
	if !ok {
		return nil, nil, false
	}
	categorized, ok := tool.(tools.CategorizedTool)
	if !ok || categorized.PermissionCategory() != tools.PermissionRead {
		return nil, nil, false
	}
	toolConn, ok := a.Connector.(connector.ToolCallingConnector)
	if !ok || !toolConn.SupportsNativeToolCalling() {
		return nil, nil, false
	}
	return tool, toolConn, true
}

// queryAskPlain performs a single-shot ask answer, streaming when requested.
func (a *Agent) queryAskPlain(ctx context.Context, sysPrompt string, opts AskOptions) (string, error) {
	qParams := connector.QueryParams{
		UserPrompt: &opts.Query,
		SysPrompt:  &sysPrompt,
		Stream:     opts.Stream,
		MaxTokens:  a.maxTokens,
		Device:     a.device,
	}
	if opts.Stream {
		qParams.OnStream = opts.OnStream
	}
	return a.Connector.Query(ctx, &qParams)
}

// emitFinalAnswer returns an answer obtained from a non-streaming tool round,
// pushing it through the stream sink once so streaming callers still render it.
func (a *Agent) emitFinalAnswer(answer string, opts AskOptions) (string, error) {
	if opts.Stream && opts.OnStream != nil && answer != "" {
		if err := opts.OnStream(answer); err != nil {
			return "", err
		}
	}
	return answer, nil
}

// webSearchQuery extracts the trimmed query string from a websearch tool input.
func webSearchQuery(input map[string]any) string {
	if q, ok := input["query"].(string); ok {
		return strings.TrimSpace(q)
	}
	return ""
}

// formatAskSearchEntry renders one search attempt for the prompt transcript.
func formatAskSearchEntry(query, result string, err error) string {
	if err != nil {
		return fmt.Sprintf("Search %q failed: %v", query, err)
	}
	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return fmt.Sprintf("Search %q returned no results.", query)
	}
	return fmt.Sprintf("Search %q results:\n%s", query, trimmed)
}

// buildAskWebSearchPrompt folds the gathered search transcript back into the
// user prompt. On the first round (empty transcript) it is just the question.
// allowMoreSearches controls whether the model is invited to search again: it
// is true during the loop and false on the final, tool-less answer turn.
func buildAskWebSearchPrompt(query string, transcript []string, allowMoreSearches bool) string {
	if len(transcript) == 0 {
		return query
	}
	var b strings.Builder
	b.WriteString(query)
	b.WriteString("\n\n<web_search_results>\n")
	b.WriteString(strings.Join(transcript, "\n\n"))
	b.WriteString("\n</web_search_results>\n\n")
	if allowMoreSearches {
		b.WriteString("Use the web search results above to answer the question. If they are insufficient, you may search again.")
	} else {
		b.WriteString("Answer the question using the web search results above. No further web searches are available.")
	}
	return b.String()
}
