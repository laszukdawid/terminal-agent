package tools

import (
	"fmt"
)

const (
	unixToolName        = "unix"
	unixToolDescription = `Unix tool provide the ability to design and run Unix commands.
	The input to the tool is a summary of the Unix command. The tool then provides Unix
	command best associated with that intent and runs the command.`

	// inputSchema = `{
	// 	"type": "object",
	// 	"properties": {
	// 		"command": {
	// 			"type": "string"
	// 		}
	// 	}
	// }`

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
    terminal-agent tool exec unix [command]
    
    Examples:
      terminal-agent tool exec unix "ls -la"
      terminal-agent tool exec unix "cat file.txt | grep pattern"
      terminal-agent tool exec unix "find . -name \"*.go\" -type f | wc -l"

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
		},
	}

	// Set default code executor
	if codeExecutor == nil {
		codeExecutor = &BashExecutor{
			confirmPrompt: true,
		}
	}

	return &UnixTool{
		name:         unixToolName,
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
	// Double check that <code>
	// Validate whether the code is a valid Unix command
	fmt.Printf("Tool: ExecCode: code: %s\n", code)
	if code == "" {
		return "", fmt.Errorf("no Unix command found in the response")
	}

	if err := validateResCode(code); err != nil {
		return "", err
	}

	// Execute the Unix command
	cmdOutput, err := u.executor.Exec(code)
	if err != nil {
		return "", fmt.Errorf("failed to execute Unix command: %v", err)
	}
	return cmdOutput, nil
}

// Run method of Unix Tool
func (u *UnixTool) Run(cmd *string) (string, error) {
	return u.ExecCode(*cmd)
}

func (u *UnixTool) RunSchema(input map[string]any) (string, error) {
	// Extract command from input
	cmd, ok := input["command"].(string)
	if !ok {
		return "", fmt.Errorf("failed to extract command from tool input")
	}

	return u.ExecCode(cmd)
}
