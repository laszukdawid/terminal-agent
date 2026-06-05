package agent

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

var readOnlyUnixCommands = map[string]struct{}{
	"cat":      {},
	"cut":      {},
	"date":     {},
	"df":       {},
	"du":       {},
	"env":      {},
	"file":     {},
	"find":     {},
	"grep":     {},
	"head":     {},
	"id":       {},
	"ls":       {},
	"printenv": {},
	"ps":       {},
	"pwd":      {},
	"rg":       {},
	"sort":     {},
	"stat":     {},
	"tail":     {},
	"tr":       {},
	"uname":    {},
	"uniq":     {},
	"wc":       {},
	"whoami":   {},
}

var unsafeFindFlags = map[string]struct{}{
	"-delete":  {},
	"-exec":    {},
	"-execdir": {},
	"-fdelete": {},
	"-ok":      {},
	"-okdir":   {},
}

func isReadOnlyUnixCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}

	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil || len(file.Stmts) != 1 {
		return false
	}

	return isReadOnlyStmt(file.Stmts[0])
}

func isReadOnlyStmt(stmt *syntax.Stmt) bool {
	if stmt == nil || stmt.Cmd == nil {
		return false
	}
	// Redirections and shell modifiers are deliberately rejected, even for input
	// redirection, to keep auto-approval limited to plain commands and pipelines.
	if stmt.Negated || stmt.Background || stmt.Coprocess || stmt.Disown || len(stmt.Redirs) > 0 {
		return false
	}

	switch cmd := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		return isReadOnlyCall(cmd)
	case *syntax.BinaryCmd:
		if cmd.Op != syntax.Pipe {
			return false
		}
		return isReadOnlyStmt(cmd.X) && isReadOnlyStmt(cmd.Y)
	default:
		// Subshells, blocks, loops, conditionals, and function declarations stay
		// outside the auto-approval surface until they have explicit handling.
		return false
	}
}

func isReadOnlyCall(call *syntax.CallExpr) bool {
	if call == nil || len(call.Assigns) > 0 || len(call.Args) == 0 {
		return false
	}

	args := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		if !isStaticShellWord(word) {
			return false
		}
		args = append(args, word.Lit())
	}

	name := args[0]
	if strings.Contains(name, "/") {
		return false
	}
	if _, ok := readOnlyUnixCommands[name]; !ok {
		return false
	}

	return readOnlyCommandAllowsArgs(name, args[1:])
}

func readOnlyCommandAllowsArgs(name string, args []string) bool {
	switch name {
	case "find":
		for _, arg := range args {
			if _, unsafe := unsafeFindFlags[arg]; unsafe {
				return false
			}
		}
	case "env":
		// env can execute a command when given operands. Variable-only operands
		// still just alter the environment display for env itself.
		for _, arg := range args {
			if !strings.Contains(arg, "=") {
				return false
			}
		}
	}
	return true
}

func isStaticShellWord(word *syntax.Word) bool {
	if word == nil || len(word.Parts) == 0 {
		return false
	}
	for _, part := range word.Parts {
		if !isStaticShellWordPart(part) {
			return false
		}
	}
	return true
}

func isStaticShellWordPart(part syntax.WordPart) bool {
	switch p := part.(type) {
	case *syntax.Lit:
		return true
	case *syntax.SglQuoted:
		return !p.Dollar
	case *syntax.DblQuoted:
		if p.Dollar {
			return false
		}
		for _, nested := range p.Parts {
			if !isStaticShellWordPart(nested) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
