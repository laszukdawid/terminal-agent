package agent

import (
	"strings"
)

type TokenKind int

const (
	TokenCommand     TokenKind = iota // first word of a segment (e.g. "find")
	TokenArg                          // positional argument (e.g. ".")
	TokenFlagPair                     // flag + value (e.g. "-maxdepth 2")
	TokenBooleanFlag                  // standalone flag (e.g. "-v")
	TokenPipeCommand                  // pipe + command name (e.g. "| sort")
)

type TokenGroup struct {
	Raw  string
	Kind TokenKind
}

func TokenizeCommand(command string) []TokenGroup {
	segments := splitPipeSegments(command)
	var groups []TokenGroup

	for i, segment := range segments {
		tokens := tokenizeRespectingQuotes(strings.TrimSpace(segment))
		if len(tokens) == 0 {
			continue
		}

		if i == 0 {
			groups = append(groups, TokenGroup{Raw: tokens[0], Kind: TokenCommand})
		} else {
			groups = append(groups, TokenGroup{Raw: "| " + tokens[0], Kind: TokenPipeCommand})
		}

		j := 1
		for j < len(tokens) {
			tok := tokens[j]
			if isFlag(tok) && j+1 < len(tokens) && !isFlag(tokens[j+1]) && tokens[j+1] != "|" {
				groups = append(groups, TokenGroup{Raw: tok + " " + tokens[j+1], Kind: TokenFlagPair})
				j += 2
			} else if isFlag(tok) {
				groups = append(groups, TokenGroup{Raw: tok, Kind: TokenBooleanFlag})
				j++
			} else {
				groups = append(groups, TokenGroup{Raw: tok, Kind: TokenArg})
				j++
			}
		}
	}

	return groups
}

func GeneratePatternLevels(groups []TokenGroup) []string {
	if len(groups) == 0 {
		return []string{"*"}
	}

	levels := make([]string, 0, len(groups))

	// Most specific (exact) first
	levels = append(levels, joinGroups(groups))

	// Each level drops one group from the right and appends *
	for i := len(groups) - 1; i >= 1; i-- {
		kept := joinGroups(groups[:i])
		levels = append(levels, kept+" *")
	}

	return levels
}


func splitPipeSegments(command string) []string {
	var segments []string
	var current strings.Builder

	depth := 0
	inSingle := false
	inDouble := false
	escape := false

	for i := 0; i < len(command); i++ {
		ch := command[i]

		if escape {
			current.WriteByte(ch)
			escape = false
			continue
		}

		if ch == '\\' && !inSingle {
			current.WriteByte(ch)
			escape = true
			continue
		}

		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
			current.WriteByte(ch)
		case ch == '"' && !inSingle:
			inDouble = !inDouble
			current.WriteByte(ch)
		case ch == '(' && !inSingle && !inDouble:
			depth++
			current.WriteByte(ch)
		case ch == ')' && !inSingle && !inDouble && depth > 0:
			depth--
			current.WriteByte(ch)
		case ch == '|' && !inSingle && !inDouble && depth == 0:
			segments = append(segments, current.String())
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		segments = append(segments, current.String())
	}

	return segments
}

func tokenizeRespectingQuotes(input string) []string {
	var tokens []string
	var current strings.Builder

	inSingle := false
	inDouble := false
	escape := false
	depth := 0

	for i := 0; i < len(input); i++ {
		ch := input[i]

		if escape {
			current.WriteByte(ch)
			escape = false
			continue
		}

		if ch == '\\' && !inSingle {
			current.WriteByte(ch)
			escape = true
			continue
		}

		switch {
		case ch == '\'' && !inDouble && depth == 0:
			inSingle = !inSingle
			current.WriteByte(ch)
		case ch == '"' && !inSingle && depth == 0:
			inDouble = !inDouble
			current.WriteByte(ch)
		case ch == '(' && !inSingle && !inDouble:
			depth++
			current.WriteByte(ch)
		case ch == ')' && !inSingle && !inDouble && depth > 0:
			depth--
			current.WriteByte(ch)
		case ch == ' ' && !inSingle && !inDouble && depth == 0:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

func isFlag(token string) bool {
	if len(token) < 2 {
		return false
	}
	if token[0] != '-' {
		return false
	}
	ch := token[1]
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '-'
}

func joinGroups(groups []TokenGroup) string {
	var b strings.Builder
	for i, g := range groups {
		if i > 0 {
			if g.Kind == TokenPipeCommand {
				b.WriteByte(' ')
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteString(g.Raw)
	}
	return b.String()
}
