package agent

import (
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const (
	awkLongOptionFile    = "--file"
	awkOptionAssign      = "-v"
	awkOptionFile        = "-f"
	awkOptionFieldSep    = "-F"
	sedLongOptionFile    = "--file"
	sedLongOptionInPlace = "--in-place"
	sedOptionExpression  = "-e"
	sedOptionFile        = "-f"
	sedOptionInPlace     = "-i"
	unixCommandCD        = "cd"
	unixBracketTestClose = "]"
	parentRelPath        = ".."
	sedSubstituteCommand = 's'
)

type readOnlyCommandPolicy struct {
	allowLoopVars bool
	validateArgs  func([]string) bool
}

var readOnlyUnixCommands = map[string]readOnlyCommandPolicy{
	":":         {},
	"[":         {validateArgs: bracketTestAllowsArgs},
	"awk":       {validateArgs: awkAllowsArgs},
	"basename":  {},
	"cat":       {},
	"cut":       {},
	"date":      {},
	"df":        {},
	"diff":      {},
	"dirname":   {},
	"du":        {},
	"echo":      {allowLoopVars: true},
	"env":       {validateArgs: envAllowsArgs},
	"false":     {validateArgs: noArgsAllowed},
	"file":      {},
	"find":      {validateArgs: findAllowsArgs},
	"grep":      {},
	"head":      {},
	"id":        {},
	"ls":        {},
	"printenv":  {},
	"printf":    {},
	"ps":        {},
	"pwd":       {},
	"realpath":  {},
	"rg":        {},
	"sed":       {validateArgs: sedAllowsArgs},
	"sha256sum": {},
	"sort":      {},
	"stat":      {},
	"tail":      {},
	"test":      {},
	"tr":        {},
	"tree":      {},
	"true":      {validateArgs: noArgsAllowed},
	"uname":     {},
	"uniq":      {},
	"wc":        {},
	"which":     {},
	"whoami":    {},
}

var unsafeFindFlags = map[string]struct{}{
	"-delete":  {},
	"-exec":    {},
	"-execdir": {},
	"-fdelete": {},
	"-ok":      {},
	"-okdir":   {},
}

var unsafeAwkProgramFragments = []string{
	"system",
	"getline",
	">",
	"|",
}

type unixSafetyContext struct {
	rootDir    string
	currentDir string
	loopVars   map[string]struct{}
}

func isReadOnlyUnixCommandInDirs(command string, dirs TaskDirs) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}

	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil || len(file.Stmts) == 0 {
		return false
	}

	ctx := unixSafetyContext{rootDir: dirs.RootDir, currentDir: dirs.CurrentDir, loopVars: map[string]struct{}{}}
	for _, stmt := range file.Stmts {
		if !isReadOnlyStmt(stmt, &ctx) {
			return false
		}
	}
	return true
}

func isReadOnlyStmt(stmt *syntax.Stmt, ctx *unixSafetyContext) bool {
	if stmt == nil || stmt.Cmd == nil {
		return false
	}
	// Redirections and shell modifiers are deliberately rejected, even for input
	// redirection, to keep auto-approval limited to parser-verified safe shell.
	if !isCleanReadOnlyStmt(stmt) {
		return false
	}

	switch cmd := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		return isReadOnlyCall(cmd, ctx, true)
	case *syntax.BinaryCmd:
		switch cmd.Op {
		case syntax.Pipe:
			return isReadOnlyPipelineStmt(cmd.X, ctx) && isReadOnlyPipelineStmt(cmd.Y, ctx)
		case syntax.AndStmt:
			return isReadOnlyStmt(cmd.X, ctx) && isReadOnlyStmt(cmd.Y, ctx)
		default:
			return false
		}
	case *syntax.ForClause:
		return isReadOnlyForClause(cmd, ctx)
	default:
		// Subshells, blocks, conditionals, function declarations, and unbounded
		// while/until loops stay outside the auto-approval surface until they have
		// explicit handling.
		return false
	}
}

func isReadOnlyPipelineStmt(stmt *syntax.Stmt, ctx *unixSafetyContext) bool {
	if stmt == nil || stmt.Cmd == nil {
		return false
	}
	if !isCleanReadOnlyStmt(stmt) {
		return false
	}

	switch cmd := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		return isReadOnlyCall(cmd, ctx, false)
	case *syntax.BinaryCmd:
		if cmd.Op != syntax.Pipe {
			return false
		}
		return isReadOnlyPipelineStmt(cmd.X, ctx) && isReadOnlyPipelineStmt(cmd.Y, ctx)
	default:
		return false
	}
}

func isCleanReadOnlyStmt(stmt *syntax.Stmt) bool {
	return stmt != nil && !stmt.Negated && !stmt.Background && !stmt.Coprocess && !stmt.Disown && len(stmt.Redirs) == 0
}

func isReadOnlyCall(call *syntax.CallExpr, ctx *unixSafetyContext, allowDirectoryChange bool) bool {
	if call == nil || len(call.Assigns) > 0 || len(call.Args) == 0 {
		return false
	}

	name, ok := staticShellWordValue(call.Args[0])
	if !ok {
		return false
	}
	if strings.Contains(name, "/") {
		return false
	}

	if isDirectoryChangeCommand(name) {
		args, ok := staticShellWordLits(call.Args)
		return ok && allowDirectoryChange && applyReadOnlyDirectoryChange(args[1:], ctx)
	}

	policy, ok := readOnlyUnixCommands[name]
	if !ok {
		return false
	}
	args := make([]string, 0, len(call.Args))
	for i, word := range call.Args {
		if i > 0 && policy.allowLoopVars {
			if !isStaticOrSafeLoopVarShellWord(word, ctx) {
				return false
			}
			args = append(args, word.Lit())
			continue
		}
		if !isStaticShellWord(word) {
			return false
		}
		arg, ok := staticShellWordValue(word)
		if !ok {
			return false
		}
		args = append(args, arg)
	}

	return readOnlyCommandAllowsArgs(policy, args[1:])
}

func isDirectoryChangeCommand(name string) bool {
	return name == unixCommandCD
}

func staticShellWordLits(words []*syntax.Word) ([]string, bool) {
	args := make([]string, 0, len(words))
	for _, word := range words {
		arg, ok := staticShellWordValue(word)
		if !ok {
			return nil, false
		}
		args = append(args, arg)
	}
	return args, true
}

func isReadOnlyForClause(clause *syntax.ForClause, ctx *unixSafetyContext) bool {
	if clause == nil || clause.Select || clause.Braces {
		return false
	}
	iter, ok := clause.Loop.(*syntax.WordIter)
	if !ok || iter == nil || iter.Name == nil || !iter.InPos.IsValid() || !isValidShellIdentifier(iter.Name.Value) {
		return false
	}
	for _, item := range iter.Items {
		if !isStaticShellWord(item) {
			return false
		}
	}
	previousLoopVars := ctx.loopVars
	ctx.loopVars = copyLoopVars(previousLoopVars)
	ctx.loopVars[iter.Name.Value] = struct{}{}
	defer func() { ctx.loopVars = previousLoopVars }()
	for range iter.Items {
		for _, stmt := range clause.Do {
			if !isReadOnlyStmt(stmt, ctx) {
				return false
			}
		}
	}
	return true
}

func applyReadOnlyDirectoryChange(args []string, ctx *unixSafetyContext) bool {
	if ctx == nil || len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return false
	}
	if ctx.rootDir == "" || ctx.currentDir == "" {
		return false
	}

	target := args[0]
	if filepath.IsAbs(target) {
		target = filepath.Clean(target)
	} else {
		target = filepath.Join(ctx.currentDir, target)
	}

	rootDir, err := filepath.Abs(ctx.rootDir)
	if err != nil {
		return false
	}
	targetDir, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rootDir, err = filepath.EvalSymlinks(rootDir)
	if err != nil {
		return false
	}
	targetDir, err = filepath.EvalSymlinks(targetDir)
	if err != nil {
		return false
	}
	info, err := os.Stat(targetDir)
	if err != nil || !info.IsDir() {
		return false
	}
	rel, err := filepath.Rel(rootDir, targetDir)
	if err != nil || rel == parentRelPath || strings.HasPrefix(rel, parentRelPath+string(os.PathSeparator)) {
		return false
	}

	ctx.currentDir = targetDir
	return true
}

func readOnlyCommandAllowsArgs(policy readOnlyCommandPolicy, args []string) bool {
	if policy.validateArgs == nil {
		return true
	}
	return policy.validateArgs(args)
}

func findAllowsArgs(args []string) bool {
	for _, arg := range args {
		if _, unsafe := unsafeFindFlags[arg]; unsafe {
			return false
		}
	}
	return true
}

func envAllowsArgs(args []string) bool {
	// env can execute a command when given operands. Variable-only operands
	// still just alter the environment display for env itself.
	for _, arg := range args {
		if !strings.Contains(arg, "=") {
			return false
		}
	}
	return true
}

func awkAllowsArgs(args []string) bool {
	if len(args) == 0 {
		return false
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == awkOptionFile || strings.HasPrefix(arg, awkOptionFile) || arg == awkLongOptionFile:
			return false
		case arg == awkOptionAssign:
			i++
			if i >= len(args) || !strings.Contains(args[i], "=") {
				return false
			}
			continue
		case strings.HasPrefix(arg, awkOptionAssign) && strings.Contains(strings.TrimPrefix(arg, awkOptionAssign), "="):
			continue
		case arg == awkOptionFieldSep:
			i++
			if i >= len(args) {
				return false
			}
			continue
		case strings.HasPrefix(arg, awkOptionFieldSep):
			continue
		case strings.HasPrefix(arg, "-"):
			return false
		}
		return awkProgramAllowsReadOnly(arg)
	}
	return false
}

func awkProgramAllowsReadOnly(program string) bool {
	for _, fragment := range unsafeAwkProgramFragments {
		if strings.Contains(program, fragment) {
			return false
		}
	}
	return true
}

func sedAllowsArgs(args []string) bool {
	if len(args) == 0 {
		return false
	}
	sawScript := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == sedOptionInPlace || strings.HasPrefix(arg, sedOptionInPlace) || strings.HasPrefix(arg, sedLongOptionInPlace):
			return false
		case arg == sedOptionFile || strings.HasPrefix(arg, sedOptionFile) || arg == sedLongOptionFile:
			return false
		case arg == sedOptionExpression:
			i++
			if i >= len(args) || !sedScriptAllowsReadOnly(args[i]) {
				return false
			}
			sawScript = true
			continue
		case isSafeSedOption(arg):
			continue
		case strings.HasPrefix(arg, "-"):
			return false
		}

		if !sawScript {
			if !sedScriptAllowsReadOnly(arg) {
				return false
			}
			sawScript = true
		}
	}
	return sawScript
}

func isSafeSedOption(arg string) bool {
	if arg == "" || arg[0] != '-' || strings.HasPrefix(arg, "--") {
		return false
	}
	for _, flag := range arg[1:] {
		switch flag {
		case 'E', 'r', 'n':
			continue
		default:
			return false
		}
	}
	return true
}

func sedScriptAllowsReadOnly(script string) bool {
	commands := strings.FieldsFunc(script, func(r rune) bool { return r == ';' || r == '\n' })
	if len(commands) == 0 {
		return false
	}
	for _, command := range commands {
		if !sedSubstitutionAllowsReadOnly(strings.TrimSpace(command)) {
			return false
		}
	}
	return true
}

func sedSubstitutionAllowsReadOnly(command string) bool {
	if len(command) < 3 || command[0] != sedSubstituteCommand {
		return false
	}
	delimiter := rune(command[1])
	if delimiter == '\\' || delimiter == '\n' {
		return false
	}
	delimiters := 0
	escaped := false
	for i, r := range command[2:] {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == delimiter {
			delimiters++
			if delimiters == 2 {
				return sedSubstitutionFlagsAllowReadOnly(command[i+3:])
			}
		}
	}
	return false
}

func sedSubstitutionFlagsAllowReadOnly(flags string) bool {
	for _, flag := range flags {
		switch {
		case flag >= '0' && flag <= '9':
			continue
		case flag == 'g' || flag == 'i' || flag == 'I' || flag == 'm' || flag == 'M' || flag == 'p':
			continue
		default:
			return false
		}
	}
	return true
}

func bracketTestAllowsArgs(args []string) bool {
	return len(args) > 0 && args[len(args)-1] == unixBracketTestClose
}

func noArgsAllowed(args []string) bool {
	return len(args) == 0
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

func staticShellWordValue(word *syntax.Word) (string, bool) {
	if word == nil || len(word.Parts) == 0 {
		return "", false
	}
	var value strings.Builder
	for _, part := range word.Parts {
		partValue, ok := staticShellWordPartValue(part)
		if !ok {
			return "", false
		}
		value.WriteString(partValue)
	}
	return value.String(), true
}

func staticShellWordPartValue(part syntax.WordPart) (string, bool) {
	switch p := part.(type) {
	case *syntax.Lit:
		return p.Value, true
	case *syntax.SglQuoted:
		return p.Value, !p.Dollar
	case *syntax.DblQuoted:
		if p.Dollar {
			return "", false
		}
		var value strings.Builder
		for _, nested := range p.Parts {
			nestedValue, ok := staticShellWordPartValue(nested)
			if !ok {
				return "", false
			}
			value.WriteString(nestedValue)
		}
		return value.String(), true
	default:
		return "", false
	}
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

func isStaticOrSafeLoopVarShellWord(word *syntax.Word, ctx *unixSafetyContext) bool {
	if word == nil || len(word.Parts) == 0 {
		return false
	}
	for _, part := range word.Parts {
		if !isStaticOrSafeLoopVarShellWordPart(part, ctx) {
			return false
		}
	}
	return true
}

func isStaticOrSafeLoopVarShellWordPart(part syntax.WordPart, ctx *unixSafetyContext) bool {
	if isStaticShellWordPart(part) {
		return true
	}
	switch p := part.(type) {
	case *syntax.ParamExp:
		return isSafeLoopVarParamExp(p, ctx)
	case *syntax.DblQuoted:
		if p.Dollar {
			return false
		}
		for _, nested := range p.Parts {
			if !isStaticOrSafeLoopVarShellWordPart(nested, ctx) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func isSafeLoopVarParamExp(exp *syntax.ParamExp, ctx *unixSafetyContext) bool {
	if exp == nil || exp.Param == nil || ctx == nil || len(ctx.loopVars) == 0 {
		return false
	}
	if exp.Flags != nil || exp.Excl || exp.Length || exp.Width || exp.IsSet || exp.NestedParam != nil || exp.Index != nil || exp.Slice != nil || exp.Repl != nil || exp.Names != 0 || exp.Exp != nil || len(exp.Modifiers) > 0 {
		return false
	}
	_, ok := ctx.loopVars[exp.Param.Value]
	return ok
}

func copyLoopVars(vars map[string]struct{}) map[string]struct{} {
	copy := make(map[string]struct{}, len(vars)+1)
	for name := range vars {
		copy[name] = struct{}{}
	}
	return copy
}

func isValidShellIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && r != '_' {
				return false
			}
			continue
		}
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}
