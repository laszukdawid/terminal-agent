package tools

import (
	"context"
	"fmt"

	log "github.com/laszukdawid/terminal-agent/internal/utils"
)

const (
	unixToolDescription = `Unix tool is for coming up with and executing Unix commands.
	One would use this tool to either execute a specific command, e.g. "ls -la",
	or to generate a command based on a description, e.g. "list all files in the
	current directory".
	It is designed to help with tasks that require Unix command line operations,
	such as file manipulation, system monitoring, and other command line tasks.
	`

	systemPrompt = `You design and execute Unix commands best associated with the intent.
	The input is provided in English and you provide the Unix command.
	Your output is in JSON form as {"lang": <LANG>, "code": <CODE>}.
	Below are some examples where the query is prefixed with "Q:".

	<examples>
	Q: What is the current directory?
	{"lang": "bash", "code": "pwd"}

	Q: Show me the current date and time?
	{"lang": "bash", "code": "date"}

	Q: List all directories in my projects directory
	{"lang": "bash", "code": "ls -l ~/projects"}
	</examples>

	Remember to provide the Unix command within <code> xml tag, and the <response> tag above is just for illustrative purposes.
	`

	unixToolHelp = `UNIX TOOL HELP
==============

NAME
    unix - Execute Unix/bash commands

DESCRIPTION
    The unix tool provides a way to execute Unix/bash commands directly from the terminal agent.
    It takes a command string as input and returns the output from executing that command.

USAGE
    agent tool exec unix [command]
    
    Examples:
      agent tool exec unix "ls -la"
      agent tool exec unix "cat file.txt | grep pattern"
      agent tool exec unix "find . -name \"*.go\" -type f | wc -l"

INPUT SCHEMA
    {
      "command": "The Unix command to execute (string)"
    }

OUTPUT
    The tool returns the standard output and standard error from executing the command.
    
SECURITY NOTES
    - Commands are executed in the current user's context
    - Use with caution as it can modify your file system
    - Commands requiring elevated privileges will fail unless the terminal agent is running with those privileges
`
)

// UnixTool Tool implements the Tool interface
type UnixTool struct {
	name         string
	description  string
	inputSchema  map[string]any
	systemPrompt string
	helpText     string

	executor CodeExecutor
}

// NewUnix returns a new UnixTool
func NewUnixTool(codeExecutor CodeExecutor) *UnixTool {

	inputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]string{
				"type": "string",
				"description": "The Unix command to execute. " +
					"Please provide the command in a single line without any new lines.",
			},
			"timeout": map[string]string{
				"type":        "string",
				"description": "Optional Go duration string bounding this command (for example 30s, 2m, 1h). Omit for the safe default; set to 0 for unlimited.",
			},
			"max_bytes": map[string]any{
				"type":        "integer",
				"description": "Optional maximum bytes of command output captured by the process runner. Omit for the safe default; set to 0 for unlimited. Live display and later task-history rendering have separate limits.",
			},
			"final": map[string]string{
				"type":        "boolean",
				"description": "Set to true only when the command output itself is the complete final user-facing answer and should be returned directly without another model summary round. Do not set for exploratory checks/listings or before create/modify tasks.",
			},
		},
		"required": []string{"command"},
	}

	// Set default code executor
	if codeExecutor == nil {
		codeExecutor = &BashExecutor{}
	}

	return &UnixTool{
		name:         ToolNameUnix,
		description:  unixToolDescription,
		inputSchema:  inputSchema,
		systemPrompt: systemPrompt,
		helpText:     unixToolHelp,
		executor:     codeExecutor,
	}
}

func (u *UnixTool) Name() string {
	return u.name
}

func (u *UnixTool) PermissionCategory() PermissionCategory {
	return PermissionExecute
}

func (u *UnixTool) Description() string {
	return u.description
}

func (u *UnixTool) InputSchema() map[string]any {
	return u.inputSchema
}

// HelpText returns detailed help information for the unix tool
func (u *UnixTool) HelpText() string {
	return u.helpText
}

func (u *UnixTool) ExecCode(code string) (string, error) {
	return u.execCodeWithExecutorContext(context.Background(), code, u.executor)
}

func (u *UnixTool) execCodeWithExecutor(code string, executor CodeExecutor) (string, error) {
	return u.execCodeWithExecutorContext(context.Background(), code, executor)
}

func (u *UnixTool) execCodeWithExecutorContext(ctx context.Context, code string, executor CodeExecutor) (string, error) {
	return u.execCodeWithExecutorOptions(ctx, code, executor, ProcessOptions{})
}

func (u *UnixTool) execCodeWithExecutorOptions(ctx context.Context, code string, executor CodeExecutor, opts ProcessOptions) (string, error) {
	log.Debugw("Executing Unix tool", "command", code)
	if code == "" {
		return "", fmt.Errorf("no Unix command found in the response")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if processExecutor, ok := executor.(ProcessOptionsCodeExecutor); ok {
		result, err := processExecutor.ExecContextWithOptions(ctx, code, opts)
		if err != nil {
			return "", fmt.Errorf("failed to execute Unix command: %w", err)
		}
		return result.Output, nil
	}
	if contextAwareExecutor, ok := executor.(ContextAwareCodeExecutor); ok {
		cmdOutput, err := contextAwareExecutor.ExecContext(ctx, code)
		if err != nil {
			return "", fmt.Errorf("failed to execute Unix command: %w", err)
		}
		return cmdOutput, nil
	} else {
		cmdOutput, err := executor.Exec(code)
		if err != nil {
			return "", fmt.Errorf("failed to execute Unix command: %w", err)
		}
		return cmdOutput, nil
	}
}

// Run method of Unix Tool
func (u *UnixTool) Run(cmd *string) (string, error) {
	return u.ExecCode(*cmd)
}

func (u *UnixTool) RunSchema(input map[string]any) (string, error) {
	return u.RunSchemaWithContext(input, ToolExecutionContext{})
}

func (u *UnixTool) RunSchemaWithContext(input map[string]any, ctx ToolExecutionContext) (string, error) {
	return u.RunSchemaContext(context.Background(), input, ctx)
}

func (u *UnixTool) RunSchemaContext(ctx context.Context, input map[string]any, execCtx ToolExecutionContext) (string, error) {
	cmd, ok := input["command"].(string)
	if !ok {
		return "", fmt.Errorf("failed to extract command from tool input")
	}
	opts, err := processOptionsFromInput(input)
	if err != nil {
		return "", err
	}

	executor := u.executor
	if execCtx.CurrentDir != "" || execCtx.Output != nil {
		workDir := execCtx.CurrentDir
		if workDir == "" {
			if bashExecutor, ok := u.executor.(*BashExecutor); ok {
				workDir = bashExecutor.workDir
			}
		}
		executor = &BashExecutor{workDir: workDir, output: execCtx.Output}
	}
	return u.execCodeWithExecutorOptions(ctx, cmd, executor, opts)
}
