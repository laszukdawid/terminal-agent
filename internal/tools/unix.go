package tools

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/utils"
)

const (
	unixToolName        = "Unix"
	unixToolDescription = `Unix tool provide the ability to design and run Unix commands.
	The input to the tool is a summary of the Unix command. The tool then provides Unix
	command best associated with that intent and runs the command.`

	systemPrompt = `You design and execute Unix commands best associated with the intent.
	The input is provided in English in and you provide the Unix command.
	The unix command has to be enclosed in <code> xml tag.
	Below are some examples where the request is provided within <ask> tag and the response is provided within <response> tag.

	<ask>What is the current directory?</ask>
	<response><code>pwd</code></response>

	<ask>Show me the current date and time?</ask>
	<response><code>date</code></response>

	<ask>List all directories in my projects directory</ask>
	<response><code>ls -l ~/projects</code></response>

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

type CodeExecutor interface {
	Exec(code string) (string, error)
}

type BashExecutor struct {
	confirmPrompt bool
	workDir       string
}

func (b *BashExecutor) Exec(code string) (string, error) {

	if b.confirmPrompt {
		// Prompt user for confirmation
		fmt.Printf("Execute the following Unix command? %s [y/N]: ", code)
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			return "", fmt.Errorf("execution cancelled by user")
		}
	}

	// Prepare command for execution
	cmd := exec.Command("bash", "-c", code)

	// Set working directory if provided
	if b.workDir != "" {
		cmd.Dir = b.workDir
	}

	// Gather cmd results
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
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

// isSupportedUnixCommand checks whether the code is a valid Unix command
// by checking the first word of the code against a list of supported Unix commands
func isSupportedUnixCommand(code string) bool {

	// How did we come up with this list?
	// Started typing a few and then Github Copilot suggested much more.
	// Cut it half, removed duplicates and obvious writes.
	validUnixCmds := []string{
		"ls", "pwd", "date", "sort", "grep", "awk", "sed", "find",
		"cat", "head", "tail", "wc", "uniq", "cut", "tr", "tee",
		"xargs", "diff", "patch", "tar", "gzip", "gunzip", "zip", "unzip",
		"curl", "wget", "ssh", "scp", "rsync", "chmod", "chown", "chgrp",
		"useradd", "usermod", "groupadd", "groupmod", "chsh", "chfn", "chage", "crontab",
		"at", "ps", "top", "free", "df", "du", "mount",
		"umount", "lsblk", "fdisk", "mkfs", "fsck", "dd", "parted", "lsof",
		"netstat", "ping", "traceroute", "dig", "host", "nslookup", "ifconfig", "ip",
		"route", "arp", "tcpdump", "wireshark", "iptables", "firewalld", "journalctl", "dmesg",
		"uname", "hostname", "uptime", "init", "systemd", "systemctl", "service",
	}

	for _, cmd := range validUnixCmds {
		if strings.Split(code, " ")[0] == cmd {
			return true
		}
	}
	return false
}

func validateResCode(res string) error {

	// Validate whether requires sudo
	if strings.Contains(res, "sudo") {
		return fmt.Errorf("command requires sudo which is not allowed")
	}

	// Validate whether the code is a valid Unix command
	if !isSupportedUnixCommand(res) {
		return fmt.Errorf("invalid Unix command found in the response")
	}

	return nil
}

func (u *UnixTool) Name() string {
	return u.name
}

func (u *UnixTool) Description() string {
	return u.description
}

// Run method of Unix Tool
func (u *UnixTool) Run(input *string) (string, error) {

	// Query the model
	res, err := u.llmClient.Query(input, &u.systemPrompt)
	if err != nil {
		return "", err
	}

	// Parse response in search for <code>
	code, err := utils.FindCodeTag(&res)
	if err != nil {
		return "", err
	}

	// Validate whether the code is a valid Unix command
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
