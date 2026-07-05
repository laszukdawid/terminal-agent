# Task Memory

The **Task Memory** tab browses the local [task memory index](../commands.md#task-memory):
the SQLite database of past task requests, tool calls, and final answers that
`agent task` searches before each model iteration. It is the same index served
by `agent memory task search` on the CLI, stored at
`~/.local/share/terminal-agent/task_memory.db`. Nothing here leaves your
machine except as part of the normal model prompts for a running task.

## Browsing and searching

Opening the tab lists the most recent **task runs**, newest first. The index
stores each run as several records (`task_request`, per-step `tool_success` /
`tool_failure`, and `task_completion`) because task retrieval ranks those
granularities differently, but for browsing they are merged into one card per
run. A card reads as intent rather than as raw index text: the user's request
appears behind the green `>` input marker, the commands the run's steps
executed behind `$`, and below them the final answer or — highlighted — the
error the run ended with. Click a card to walk the whole run: the request
once, then each indexed record in execution order with its command, the
model's thought, and its full output, answer, or error, plus the source run id
so you can trace it back to its [session log](./history.md). Press
<kbd>Esc</kbd> or **Close** to dismiss the detail view.

Type into the search field and press <kbd>Enter</kbd> (or **SEARCH**) to query
the index with the same ranking the task agent uses: full-text search when
SQLite FTS5 is available, with actionable `tool_success` records preferred and
records from your current operating system boosted. Search results list
individual records rather than merged runs, because a match is a specific
record and its rank; their detail view additionally shows the complete indexed
text exactly as the search engine sees it. Clearing the field returns to the
recent-runs view.

The line under the list reports the index size, how much the current view
shows (runs when browsing, matched records when searching), and whether
full-text search is active.

## Clearing the index

**CLEAR** deletes every indexed record after a confirmation. This only resets
what future tasks can recall; the session logs under
`~/.local/share/terminal-agent/sessions/` are untouched and remain the
authoritative history of what ran.

If `task_memory` is disabled in the [configuration](../configuration.md), the
tab still browses existing records but shows a notice that new task runs are
not being indexed.
