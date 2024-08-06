package tools

import (
	"fmt"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/utils"
)

const (
	unixToolName        = "unix"
	unixToolDescription = `Unix tool provide the ability to design and run Unix commands.
	The input to the tool is a summary of the Unix command. The tool then provides Unix
	command best associated with that intent and runs the command.`

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
)

// UnixTool Tool implements the Tool interface
type UnixTool struct {
	name         string
	description  string
	systemPrompt string

	llmClient connector.LLMConnector
	executor  CodeExecutor
}

// NewUnix returns a new UnixTool
func NewUnixTool(llmClient connector.LLMConnector, codeExecutor CodeExecutor) *UnixTool {

	// Set default code executor
	if codeExecutor == nil {
		codeExecutor = &BashExecutor{
			confirmPrompt: true,
		}
	}

	return &UnixTool{
		name:         unixToolName,
		description:  unixToolDescription,
		systemPrompt: systemPrompt,
		llmClient:    llmClient,
		executor:     codeExecutor,
	}
}

func (u *UnixTool) Name() string {
	return u.name
}

func (u *UnixTool) Description() string {
	return u.description
}

func (u *UnixTool) DetectTool(input *string) bool {
	// TODO: ????

	return false
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
func (u *UnixTool) Run(input *string) (string, error) {
	sugar := utils.Logger.Sugar()

	// Query the model with provided instruction
	res, err := u.llmClient.Query(input, &u.systemPrompt)
	sugar.Debugw("Result of Query", "res", res, "err", err, "input", *input)
	if err != nil {
		return "", err
	}

	// Parse response for `codeObject`
	codeObject, err := utils.FindCodeObject(&res)
	sugar.Debugw("Result of FindCodeObject", "codeObject", codeObject, "err", err)
	if err != nil {
		return "", err
	}

	return u.ExecCode(codeObject.Code)
}
