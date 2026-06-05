package commands

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

const (
	taskTraceANSIBoldCyan = "\x1b[1;36m"
	taskTraceANSIBold     = "\x1b[1m"
	taskTraceANSIReset    = "\x1b[0m"
)

func isTerminalWriter(out io.Writer) bool {
	file, ok := out.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func formatTaskTrace(label string, value string, styled bool) string {
	if !styled {
		return fmt.Sprintf("%s: %s", label, value)
	}
	return fmt.Sprintf("%s%s:%s %s%s%s", taskTraceANSIBoldCyan, label, taskTraceANSIReset, taskTraceANSIBold, value, taskTraceANSIReset)
}
