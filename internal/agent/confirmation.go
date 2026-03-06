package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/config"
)

type ConfirmationManager struct {
	allowPatterns []rulePattern
	denyPatterns  []rulePattern
	askPatterns   []rulePattern
	decisions     map[string]bool
	rememberFunc  RememberDecisionFunc
}

type RememberDecisionFunc func(action string, allow bool) error

type ruleType int

const (
	ruleAllow ruleType = iota
	ruleDeny
	ruleAsk
)

type rulePattern struct {
	pattern  allowPattern
	priority int
}

type allowPattern struct {
	tool        string
	command     *regexp.Regexp
	argPatterns map[string]*regexp.Regexp
	allowKeys   []*regexp.Regexp
}

type actionCall struct {
	tool    string
	command string
	args    map[string]string
}

func NewConfirmationManager(allow []string, ruleSets []config.PermissionRuleSet, remember RememberDecisionFunc) *ConfirmationManager {
	manager := &ConfirmationManager{
		decisions:    make(map[string]bool),
		rememberFunc: remember,
	}

	maxPriority := 0
	for _, set := range ruleSets {
		if set.Priority > maxPriority {
			maxPriority = set.Priority
		}
		manager.appendPatterns(set.Permissions.Allow, ruleAllow, set.Priority)
		manager.appendPatterns(set.Permissions.Deny, ruleDeny, set.Priority)
		manager.appendPatterns(set.Permissions.Ask, ruleAsk, set.Priority)
	}

	manager.appendPatterns(allow, ruleAllow, maxPriority+1)

	return manager
}

func (cm *ConfirmationManager) Confirm(action string) (bool, error) {
	if action == "" {
		return true, nil
	}

	if decision, ok := cm.decisions[action]; ok {
		return decision, nil
	}

	if cm.shouldAsk(action) {
		return cm.promptAndRemember(action)
	}

	if allowed, matched := cm.resolveAllowDeny(action); matched {
		cm.decisions[action] = allowed
		return allowed, nil
	}

	return cm.promptAndRemember(action)
}

func (cm *ConfirmationManager) appendPatterns(values []string, rule ruleType, priority int) {
	for _, entry := range normalizeList(values) {
		pattern, err := parseAllowPattern(entry)
		if err != nil {
			continue
		}
		rulePattern := rulePattern{
			pattern:  pattern,
			priority: priority,
		}
		switch rule {
		case ruleAllow:
			cm.allowPatterns = append(cm.allowPatterns, rulePattern)
		case ruleDeny:
			cm.denyPatterns = append(cm.denyPatterns, rulePattern)
		case ruleAsk:
			cm.askPatterns = append(cm.askPatterns, rulePattern)
		}
	}
}

func (cm *ConfirmationManager) shouldAsk(action string) bool {
	return cm.matchesPatterns(action, cm.askPatterns)
}

func (cm *ConfirmationManager) resolveAllowDeny(action string) (bool, bool) {
	allowMatch, allowPriority := cm.matchWithPriority(action, cm.allowPatterns)
	denyMatch, denyPriority := cm.matchWithPriority(action, cm.denyPatterns)

	if !allowMatch && !denyMatch {
		return false, false
	}
	if allowMatch && denyMatch {
		if denyPriority >= allowPriority {
			return false, true
		}
		return true, true
	}
	if denyMatch {
		return false, true
	}
	return true, true
}

func (cm *ConfirmationManager) promptAndRemember(action string) (bool, error) {
	decision, err := promptConfirmation(action)
	if err != nil {
		return false, err
	}

	cm.decisions[action] = decision.allowed
	if decision.remember && cm.rememberFunc != nil {
		if err := cm.rememberFunc(action, decision.allowed); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: unable to remember permission for %s: %v\n", action, err)
		}
	}

	return decision.allowed, nil
}

func (cm *ConfirmationManager) matchWithPriority(action string, patterns []rulePattern) (bool, int) {
	call, err := parseActionCall(action)
	if err != nil {
		return false, 0
	}

	matched := false
	priority := 0
	for _, pattern := range patterns {
		if pattern.pattern.tool != call.tool {
			continue
		}
		if pattern.pattern.command != nil && !pattern.pattern.command.MatchString(call.command) {
			continue
		}
		if !pattern.pattern.matchArgs(call.args) {
			continue
		}
		if !matched || pattern.priority > priority {
			matched = true
			priority = pattern.priority
		}
	}

	return matched, priority
}

func (cm *ConfirmationManager) matchesPatterns(action string, patterns []rulePattern) bool {
	call, err := parseActionCall(action)
	if err != nil {
		return false
	}

	for _, pattern := range patterns {
		if pattern.pattern.tool != call.tool {
			continue
		}
		if pattern.pattern.command != nil && !pattern.pattern.command.MatchString(call.command) {
			continue
		}
		if !pattern.pattern.matchArgs(call.args) {
			continue
		}
		return true
	}

	return false
}

func normalizeList(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	return normalized
}

type confirmationDecision struct {
	allowed  bool
	remember bool
}

func promptConfirmation(action string) (confirmationDecision, error) {
	fmt.Printf("Execute the following action?\n > %s [y/N/yes!/no!]: ", action)
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return confirmationDecision{}, err
	}
	response = strings.TrimSpace(strings.ToLower(response))

	switch response {
	case "y", "yes":
		return confirmationDecision{allowed: true}, nil
	case "yes!":
		return confirmationDecision{allowed: true, remember: true}, nil
	case "no!":
		return confirmationDecision{allowed: false, remember: true}, nil
	default:
		return confirmationDecision{allowed: false}, nil
	}
}

func BuildActionString(toolName string, input map[string]any) string {
	if toolName == "" {
		return ""
	}

	parts := make([]string, 0, len(input)+1)
	if command, ok := input["command"]; ok {
		parts = append(parts, formatActionValue(command))
	}

	keys := make([]string, 0, len(input))
	for key := range input {
		if key == "command" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, formatActionValue(input[key])))
	}

	return fmt.Sprintf("%s(%s)", toolName, strings.Join(parts, ", "))
}

func formatActionValue(value any) string {
	if value == nil {
		return "\"\""
	}
	switch v := value.(type) {
	case string:
		return quoteString(v)
	case []string:
		return formatStringSlice(v)
	case []any:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, formatActionValue(item))
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ", "))
	case map[string]any:
		return mustJSON(v)
	default:
		return fmt.Sprint(value)
	}
}

func formatStringSlice(values []string) string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		items = append(items, quoteString(value))
	}
	return fmt.Sprintf("[%s]", strings.Join(items, ", "))
}

func quoteString(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"\n", "\\n",
		"\r", "\\r",
		"\t", "\\t",
	)
	return fmt.Sprintf("\"%s\"", replacer.Replace(value))
}

func mustJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "\"\""
	}
	return string(encoded)
}

func (pattern allowPattern) matchArgs(args map[string]string) bool {
	for key, matcher := range pattern.argPatterns {
		value, ok := args[key]
		if !ok {
			return false
		}
		if !matcher.MatchString(value) {
			return false
		}
	}

	if len(pattern.allowKeys) == 0 {
		return true
	}

	for key := range args {
		if !matchesAnyRegex(key, pattern.allowKeys) {
			return false
		}
	}

	return true
}

func matchesAnyRegex(value string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func parseAllowPattern(input string) (allowPattern, error) {
	tool, command, args, allowKeys, err := parseActionExpression(input)
	if err != nil {
		return allowPattern{}, err
	}

	pattern := allowPattern{
		tool:        tool,
		argPatterns: make(map[string]*regexp.Regexp),
	}

	if command != "" {
		regex, err := compileRegex(command)
		if err != nil {
			return allowPattern{}, err
		}
		pattern.command = regex
	}

	for key, value := range args {
		regex, err := compileRegex(value)
		if err != nil {
			return allowPattern{}, err
		}
		pattern.argPatterns[key] = regex
	}

	for _, keyPattern := range allowKeys {
		regex, err := compileRegex(keyPattern)
		if err != nil {
			return allowPattern{}, err
		}
		pattern.allowKeys = append(pattern.allowKeys, regex)
	}

	return pattern, nil
}

func parseActionCall(input string) (actionCall, error) {
	tool, command, args, _, err := parseActionExpression(input)
	if err != nil {
		return actionCall{}, err
	}

	return actionCall{
		tool:    tool,
		command: command,
		args:    args,
	}, nil
}

func parseActionExpression(input string) (string, string, map[string]string, []string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", nil, nil, fmt.Errorf("empty action")
	}

	open := strings.Index(input, "(")
	close := strings.LastIndex(input, ")")
	if open == -1 || close == -1 || close < open {
		return "", "", nil, nil, fmt.Errorf("invalid action format")
	}

	tool := strings.TrimSpace(input[:open])
	if tool == "" {
		return "", "", nil, nil, fmt.Errorf("missing tool name")
	}

	argsSection := strings.TrimSpace(input[open+1 : close])
	args := make(map[string]string)
	allowKeys := []string{}
	command := ""

	if argsSection == "" {
		return tool, command, args, allowKeys, nil
	}

	items, err := splitTopLevel(argsSection)
	if err != nil {
		return "", "", nil, nil, err
	}

	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}

		key, value, hasKey := splitKeyValue(item)
		if !hasKey {
			if command != "" {
				return "", "", nil, nil, fmt.Errorf("multiple positional arguments")
			}
			command, err = parseValueString(item)
			if err != nil {
				return "", "", nil, nil, err
			}
			continue
		}

		if key == "allowKeys" {
			allowKeys, err = parseValueList(value)
			if err != nil {
				return "", "", nil, nil, err
			}
			continue
		}

		parsed, err := parseValueString(value)
		if err != nil {
			return "", "", nil, nil, err
		}
		args[key] = parsed
	}

	return tool, command, args, allowKeys, nil
}

func splitTopLevel(input string) ([]string, error) {
	var items []string
	var current strings.Builder

	depth := 0
	inQuotes := false
	escape := false

	for _, r := range input {
		switch {
		case escape:
			current.WriteRune(r)
			escape = false
		case r == '\\' && inQuotes:
			current.WriteRune(r)
			escape = true
		case r == '"':
			current.WriteRune(r)
			inQuotes = !inQuotes
		case r == '[' && !inQuotes:
			depth++
			current.WriteRune(r)
		case r == ']' && !inQuotes:
			if depth == 0 {
				return nil, fmt.Errorf("unexpected closing bracket")
			}
			depth--
			current.WriteRune(r)
		case r == ',' && !inQuotes && depth == 0:
			items = append(items, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}

	if inQuotes || depth != 0 {
		return nil, fmt.Errorf("unterminated expression")
	}

	if current.Len() > 0 {
		items = append(items, current.String())
	}

	return items, nil
}

func splitKeyValue(input string) (string, string, bool) {
	depth := 0
	inQuotes := false
	escape := false

	for i, r := range input {
		switch {
		case escape:
			escape = false
		case r == '\\' && inQuotes:
			escape = true
		case r == '"':
			inQuotes = !inQuotes
		case r == '[' && !inQuotes:
			depth++
		case r == ']' && !inQuotes && depth > 0:
			depth--
		case r == '=' && !inQuotes && depth == 0:
			key := strings.TrimSpace(input[:i])
			value := strings.TrimSpace(input[i+1:])
			return key, value, true
		}
	}

	return "", "", false
}

func parseValueString(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil
	}
	if strings.HasPrefix(input, "[") {
		items, err := parseValueList(input)
		if err != nil {
			return "", err
		}
		return strings.Join(items, ","), nil
	}
	if strings.HasPrefix(input, "\"") {
		return unquoteString(input)
	}
	return input, nil
}

func parseValueList(input string) ([]string, error) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "[") || !strings.HasSuffix(input, "]") {
		return nil, fmt.Errorf("expected list")
	}
	content := strings.TrimSpace(input[1 : len(input)-1])
	if content == "" {
		return []string{}, nil
	}

	items, err := splitTopLevel(content)
	if err != nil {
		return nil, err
	}

	results := make([]string, 0, len(items))
	for _, item := range items {
		value, err := parseValueString(item)
		if err != nil {
			return nil, err
		}
		results = append(results, value)
	}

	return results, nil
}

func unquoteString(input string) (string, error) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "\"") {
		return input, nil
	}
	value, err := strconv.Unquote(input)
	if err != nil {
		return "", err
	}
	return value, nil
}

func compileRegex(pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, fmt.Errorf("empty regex")
	}
	return regexp.Compile(fmt.Sprintf("^(?:%s)$", pattern))
}
