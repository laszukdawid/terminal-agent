package tools

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type BashExecutor struct {
	confirmPrompt bool
	workDir       string
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
		return fmt.Errorf("invalid unix command: %s", res)
	}

	return nil
}

func (b *BashExecutor) Exec(code string) (string, error) {

	if b.confirmPrompt {
		// Prompt user for confirmation
		fmt.Printf("Execute the following Unix command?\n > %s [y/N]: ", code)
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			return "", nil
		}
	}

	// Prepare command for execution
	fmt.Printf("Executing Unix command: %s\n", code)
	cmd := exec.Command("bash", "-c", code)

	// Set working directory if provided
	if b.workDir != "" {
		fmt.Printf("Working directory: %s\n", b.workDir)
		cmd.Dir = b.workDir
	}

	// Gather cmd results
	output, err := cmd.CombinedOutput()
	strOutput := string(output)
	fmt.Printf("Output: %s\n", strOutput)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(strOutput), nil
}
