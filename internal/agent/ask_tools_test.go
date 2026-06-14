package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeWebSearchTool is a read-category stand-in for the websearch tool. It
// returns scripted outputs (or an injected error) and records its inputs.
type fakeWebSearchTool struct {
	outputs []string
	err     error
	calls   int
	inputs  []map[string]any
}

func (f *fakeWebSearchTool) Name() string { return tools.ToolNameWebsearch }
func (f *fakeWebSearchTool) PermissionCategory() tools.PermissionCategory {
	return tools.PermissionRead
}
func (f *fakeWebSearchTool) Description() string         { return "" }
func (f *fakeWebSearchTool) InputSchema() map[string]any { return map[string]any{} }
func (f *fakeWebSearchTool) HelpText() string            { return "" }
func (f *fakeWebSearchTool) Run(*string) (string, error) { return f.RunSchema(nil) }
func (f *fakeWebSearchTool) RunSchema(input map[string]any) (string, error) {
	f.inputs = append(f.inputs, input)
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	if len(f.outputs) == 0 {
		return "", nil
	}
	idx := f.calls - 1
	if idx >= len(f.outputs) {
		idx = len(f.outputs) - 1
	}
	return f.outputs[idx], nil
}

func newWebSearchAgent(conn connector.LLMConnector, tool tools.Tool) *Agent {
	askPrompt := SystemPromptAsk
	toolSet := map[string]tools.Tool{}
	if tool != nil {
		toolSet[tool.Name()] = tool
	}
	return &Agent{
		Connector:       conn,
		Tools:           toolSet,
		systemPromptAsk: &askPrompt,
		maxTokens:       MaxTokens,
	}
}

func toolCall(query string) connector.LlmResponseWithTools {
	return connector.LlmResponseWithTools{
		ToolUse:   true,
		ToolName:  tools.ToolNameWebsearch,
		ToolInput: map[string]any{"query": query},
	}
}

func answer(text string) connector.LlmResponseWithTools {
	return connector.LlmResponseWithTools{Response: text}
}

func TestQuestionWithWebSearch(t *testing.T) {
	t.Run("intent off skips tool loop and answers plainly", func(t *testing.T) {
		conn := &scriptedToolConnector{queryResponse: "plain answer"}
		ws := &fakeWebSearchTool{outputs: []string{"unused"}}
		agent := newWebSearchAgent(conn, ws)

		got, err := agent.QuestionWithWebSearch(context.Background(), AskOptions{
			Query:        "what time is it",
			UseWebSearch: false,
		})

		require.NoError(t, err)
		assert.Equal(t, "plain answer", got)
		assert.Equal(t, 0, conn.queryToolCalls, "tool calling must not be attempted")
		assert.Equal(t, 1, conn.queryCalls, "plain query is used")
		assert.Equal(t, 0, ws.calls, "websearch must not run when intent is off")
	})

	t.Run("connector without native tool calling falls back to plain", func(t *testing.T) {
		conn := &fallbackTaskConnector{queryResponse: "fallback answer"}
		ws := &fakeWebSearchTool{outputs: []string{"unused"}}
		agent := newWebSearchAgent(conn, ws)

		got, err := agent.QuestionWithWebSearch(context.Background(), AskOptions{
			Query:        "latest go version",
			UseWebSearch: true,
		})

		require.NoError(t, err)
		assert.Equal(t, "fallback answer", got)
		assert.Equal(t, 0, conn.toolCalls, "QueryWithTool must not be called")
		assert.Equal(t, 0, ws.calls)
	})

	t.Run("missing websearch tool falls back to plain", func(t *testing.T) {
		conn := &scriptedToolConnector{queryResponse: "plain answer"}
		agent := newWebSearchAgent(conn, nil)

		got, err := agent.QuestionWithWebSearch(context.Background(), AskOptions{
			Query:        "latest go version",
			UseWebSearch: true,
		})

		require.NoError(t, err)
		assert.Equal(t, "plain answer", got)
		assert.Equal(t, 0, conn.queryToolCalls)
	})

	t.Run("single search round then answer", func(t *testing.T) {
		conn := &scriptedToolConnector{responses: []connector.LlmResponseWithTools{
			toolCall("go 1.30 release date"),
			answer("Go 1.30 was released."),
		}}
		ws := &fakeWebSearchTool{outputs: []string{"- [Go 1.30](https://go.dev)"}}
		agent := newWebSearchAgent(conn, ws)

		got, err := agent.QuestionWithWebSearch(context.Background(), AskOptions{
			Query:        "when was go 1.30 released",
			UseWebSearch: true,
		})

		require.NoError(t, err)
		assert.Equal(t, "Go 1.30 was released.", got)
		assert.Equal(t, 1, ws.calls)
		assert.Equal(t, 2, conn.queryToolCalls)
		assert.Equal(t, 0, conn.queryCalls, "no extra plain query when model answers in-loop")
		// The second tool turn must carry the search results back to the model.
		require.Len(t, conn.toolPrompts, 2)
		assert.Contains(t, conn.toolPrompts[1], "web_search_results")
		assert.Contains(t, conn.toolPrompts[1], "go.dev")
	})

	t.Run("search failure is recorded and the model still answers", func(t *testing.T) {
		conn := &scriptedToolConnector{responses: []connector.LlmResponseWithTools{
			toolCall("flaky query"),
			answer("Answer despite failed search."),
		}}
		ws := &fakeWebSearchTool{err: errors.New("tavily unavailable")}
		agent := newWebSearchAgent(conn, ws)

		var steps []AskStep
		got, err := agent.QuestionWithWebSearch(context.Background(), AskOptions{
			Query:        "something",
			UseWebSearch: true,
			OnStep:       func(s AskStep) { steps = append(steps, s) },
		})

		require.NoError(t, err)
		assert.Equal(t, "Answer despite failed search.", got)
		assert.Equal(t, 1, ws.calls)
		require.Len(t, steps, 1)
		require.Error(t, steps[0].Err)
		assert.Contains(t, conn.toolPrompts[1], "failed")
	})

	t.Run("iteration cap forces a final plain answer", func(t *testing.T) {
		conn := &scriptedToolConnector{
			responses: []connector.LlmResponseWithTools{
				toolCall("q1"),
				toolCall("q2"),
				toolCall("q3"),
			},
			queryResponse: "final answer after cap",
		}
		ws := &fakeWebSearchTool{outputs: []string{"r1", "r2", "r3"}}
		agent := newWebSearchAgent(conn, ws)

		got, err := agent.QuestionWithWebSearch(context.Background(), AskOptions{
			Query:        "deep question",
			UseWebSearch: true,
		})

		require.NoError(t, err)
		assert.Equal(t, "final answer after cap", got)
		assert.Equal(t, maxAskWebSearchRounds, ws.calls)
		assert.Equal(t, maxAskWebSearchRounds, conn.queryToolCalls)
		assert.Equal(t, 1, conn.queryCalls, "one final plain query closes out the run")
	})

	t.Run("repeated identical query stops the loop", func(t *testing.T) {
		conn := &scriptedToolConnector{
			responses: []connector.LlmResponseWithTools{
				toolCall("same query"),
				toolCall("same query"),
			},
			queryResponse: "final answer after repeat",
		}
		ws := &fakeWebSearchTool{outputs: []string{"r1"}}
		agent := newWebSearchAgent(conn, ws)

		got, err := agent.QuestionWithWebSearch(context.Background(), AskOptions{
			Query:        "deep question",
			UseWebSearch: true,
		})

		require.NoError(t, err)
		assert.Equal(t, "final answer after repeat", got)
		assert.Equal(t, 1, ws.calls, "the repeated search is not executed again")
		assert.Equal(t, 2, conn.queryToolCalls)
		assert.Equal(t, 1, conn.queryCalls)
	})

	t.Run("streaming emits the final answer once on the tool path", func(t *testing.T) {
		conn := &scriptedToolConnector{responses: []connector.LlmResponseWithTools{
			toolCall("q"),
			answer("streamed answer"),
		}}
		ws := &fakeWebSearchTool{outputs: []string{"r"}}
		agent := newWebSearchAgent(conn, ws)

		var chunks []string
		got, err := agent.QuestionWithWebSearch(context.Background(), AskOptions{
			Query:        "q",
			UseWebSearch: true,
			Stream:       true,
			OnStream:     func(c string) error { chunks = append(chunks, c); return nil },
		})

		require.NoError(t, err)
		assert.Equal(t, "streamed answer", got)
		assert.Equal(t, []string{"streamed answer"}, chunks)
	})

	t.Run("non-websearch tool call is never executed", func(t *testing.T) {
		conn := &scriptedToolConnector{
			responses: []connector.LlmResponseWithTools{
				{ToolUse: true, ToolName: "python", ToolInput: map[string]any{"code": "print(1)"}},
			},
			queryResponse: "answer without running other tools",
		}
		ws := &fakeWebSearchTool{outputs: []string{"unused"}}
		agent := newWebSearchAgent(conn, ws)

		got, err := agent.QuestionWithWebSearch(context.Background(), AskOptions{
			Query:        "do something",
			UseWebSearch: true,
		})

		require.NoError(t, err)
		assert.Equal(t, "answer without running other tools", got)
		assert.Equal(t, 0, ws.calls, "only websearch may run on the ask path")
		assert.Equal(t, 1, conn.queryCalls, "falls through to a final plain answer")
	})

	t.Run("empty or whitespace query is not searched", func(t *testing.T) {
		cases := map[string]map[string]any{
			"missing query":    {},
			"whitespace query": {"query": "   "},
			"non-string query": {"query": 42},
		}
		for name, input := range cases {
			t.Run(name, func(t *testing.T) {
				conn := &scriptedToolConnector{
					responses: []connector.LlmResponseWithTools{
						{ToolUse: true, ToolName: tools.ToolNameWebsearch, ToolInput: input},
					},
					queryResponse: "final answer",
				}
				ws := &fakeWebSearchTool{outputs: []string{"unused"}}
				agent := newWebSearchAgent(conn, ws)

				got, err := agent.QuestionWithWebSearch(context.Background(), AskOptions{
					Query:        "deep question",
					UseWebSearch: true,
				})

				require.NoError(t, err)
				assert.Equal(t, "final answer", got)
				assert.Equal(t, 0, ws.calls, "an empty query must not reach the search API")
			})
		}
	})

	t.Run("canceled context stops before searching", func(t *testing.T) {
		conn := &scriptedToolConnector{responses: []connector.LlmResponseWithTools{toolCall("q")}}
		ws := &fakeWebSearchTool{outputs: []string{"r"}}
		agent := newWebSearchAgent(conn, ws)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := agent.QuestionWithWebSearch(ctx, AskOptions{Query: "q", UseWebSearch: true})
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 0, ws.calls)
	})

	t.Run("stream sink error is propagated", func(t *testing.T) {
		conn := &scriptedToolConnector{responses: []connector.LlmResponseWithTools{
			toolCall("q"),
			answer("streamed answer"),
		}}
		ws := &fakeWebSearchTool{outputs: []string{"r"}}
		agent := newWebSearchAgent(conn, ws)

		streamErr := errors.New("sink closed")
		_, err := agent.QuestionWithWebSearch(context.Background(), AskOptions{
			Query:        "q",
			UseWebSearch: true,
			Stream:       true,
			OnStream:     func(string) error { return streamErr },
		})
		assert.ErrorIs(t, err, streamErr)
	})

	t.Run("empty query is rejected", func(t *testing.T) {
		agent := newWebSearchAgent(&scriptedToolConnector{}, nil)
		_, err := agent.QuestionWithWebSearch(context.Background(), AskOptions{Query: "  "})
		assert.ErrorIs(t, err, ErrEmptyQuery)
	})
}

func TestBuildAskWebSearchPrompt(t *testing.T) {
	t.Run("no transcript returns the bare query", func(t *testing.T) {
		assert.Equal(t, "my question", buildAskWebSearchPrompt("my question", nil, true))
	})

	t.Run("transcript is wrapped and appended", func(t *testing.T) {
		got := buildAskWebSearchPrompt("my question", []string{"Search \"x\" results:\n- a"}, true)
		assert.True(t, strings.HasPrefix(got, "my question"))
		assert.Contains(t, got, "<web_search_results>")
		assert.Contains(t, got, "</web_search_results>")
		assert.Contains(t, got, "- a")
		assert.Contains(t, got, "you may search again")
	})

	t.Run("final turn does not invite more searches", func(t *testing.T) {
		got := buildAskWebSearchPrompt("my question", []string{"Search \"x\" results:\n- a"}, false)
		assert.Contains(t, got, "No further web searches are available")
		assert.NotContains(t, got, "you may search again")
	})
}
