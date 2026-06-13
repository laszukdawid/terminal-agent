package agent

import (
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/tools"
)

func TestConfirmWithDefaultAutoAllowFallback(t *testing.T) {
	manager := NewConfirmationManager(nil, nil, nil, nil)

	allowed, err := manager.ConfirmWithDefault("read(path=\"/tmp/file\")", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("autoAllow fallback should allow without prompting")
	}
}

func TestConfirmWithDefaultGatesWhenNotAutoAllowed(t *testing.T) {
	manager := NewConfirmationManager(nil, nil, nil, nil)

	// A nil confirm callback errors when a prompt is required, which is how we
	// detect that the action was gated rather than auto-allowed.
	if _, err := manager.ConfirmWithDefault("unix(\"rm -rf /\")", false); err == nil {
		t.Fatal("expected a prompt when not auto-allowed and no rule matches")
	}
}

func TestConfirmWithDefaultDenyBeatsAutoAllow(t *testing.T) {
	manager := NewConfirmationManager(nil, []config.PermissionRuleSet{{
		Permissions: config.Permissions{Deny: []string{"file_edit(path=\"*\")"}},
	}}, nil, nil)

	allowed, err := manager.ConfirmWithDefault("file_edit(path=\"/repo/x\")", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("deny rule must override autoAllow")
	}
}

func TestConfirmWithDefaultAskBeatsAutoAllow(t *testing.T) {
	manager := NewConfirmationManager(nil, []config.PermissionRuleSet{{
		Permissions: config.Permissions{Ask: []string{"file_edit(path=\"*\")"}},
	}}, nil, nil)

	if _, err := manager.ConfirmWithDefault("file_edit(path=\"/repo/x\")", true); err == nil {
		t.Fatal("ask rule must force a prompt even when autoAllow is true")
	}
}

func TestConfirmWithPolicyAutoApproveSkipsAsk(t *testing.T) {
	manager := NewConfirmationManager(nil, []config.PermissionRuleSet{{
		Permissions: config.Permissions{Ask: []string{"unix(\"aws *\")"}},
	}}, nil, nil)

	allowed, err := manager.ConfirmWithPolicy("unix(\"aws sso login\")", false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("autoApprove should allow when only ask rules match")
	}
}

func TestConfirmWithPolicyAutoApproveRespectsDeny(t *testing.T) {
	manager := NewConfirmationManager(nil, []config.PermissionRuleSet{{
		Permissions: config.Permissions{Deny: []string{"unix(\"rm *\")"}},
	}}, nil, nil)

	allowed, err := manager.ConfirmWithPolicy("unix(\"rm file\")", false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("autoApprove must not override deny rules")
	}
}

func TestBuildActionString(t *testing.T) {
	action := BuildActionString(tools.ToolNameUnix, map[string]any{
		"command":   "aws login sso",
		"final":     true,
		"timeout":   "2m",
		"max_bytes": 65536,
		"flag":      "value",
	})

	if action != tools.ToolNameUnix+"(\"aws login sso\", flag=\"value\")" {
		t.Fatalf("unexpected action string: %s", action)
	}
}

func TestParseToolAndDisplay(t *testing.T) {
	tests := []struct {
		name        string
		action      string
		wantTool    string
		wantDisplay string
	}{
		{
			name:        "python code",
			action:      tools.ToolNamePython + `(code="print(\"hello\")\nprint(\"world\")")`,
			wantTool:    tools.ToolNamePython,
			wantDisplay: "print(\"hello\")\nprint(\"world\")",
		},
		{
			name:        "python path",
			action:      tools.ToolNamePython + `(path="scripts/report.py")`,
			wantTool:    tools.ToolNamePython,
			wantDisplay: "scripts/report.py",
		},
		{
			name:        "unix command",
			action:      tools.ToolNameUnix + `("git status")`,
			wantTool:    tools.ToolNameUnix,
			wantDisplay: "git status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, display := ParseToolAndDisplay(tt.action)
			if tool != tt.wantTool {
				t.Fatalf("unexpected tool: %s", tool)
			}
			if display != tt.wantDisplay {
				t.Fatalf("unexpected display: %q", display)
			}
		})
	}
}

func TestConfirmationAllowsExactAndGlob(t *testing.T) {
	manager := NewConfirmationManager([]string{
		"unix(\"aws login\")",
		"unix(\"cat *\")",
	}, nil, nil, nil)

	allowed, err := manager.Confirm("unix(\"aws login\")")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatalf("expected exact allow to pass")
	}

	allowed, err = manager.Confirm("unix(\"cat /etc/hosts\")")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatalf("expected glob allow to pass")
	}
}

func TestAllowKeysGlob(t *testing.T) {
	manager := NewConfirmationManager([]string{
		"unix(\"aws login\", allowKeys=[\"region\", \"profile\", \"read*\"])",
		"unix(\"aws login\", region=\"us-*\")",
	}, nil, nil, nil)

	allowed, err := manager.Confirm("unix(\"aws login\")")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatalf("expected allowKeys match to pass with no keys")
	}

	allowed, err = manager.Confirm("unix(\"aws login\", region=\"us-west-2\")")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatalf("expected allowKeys match to pass with allowed key")
	}

	allowed, err = manager.Confirm("unix(\"aws login\", write=\"foo\")")
	if err == nil {
		t.Fatalf("expected prompt to be required for unknown key")
	}

	allowed, matched := manager.resolveAllowDeny("unix(\"aws login\", write=\"foo\")")
	if matched || allowed {
		t.Fatalf("expected allowKeys to reject unknown key")
	}
}

func TestRememberedPermissionMatchesExactAction(t *testing.T) {
	manager := NewConfirmationManager([]string{
		"unix(\"ls -d */\", path=\"./plans/[draft].md\")",
	}, nil, nil, nil)

	allowed, err := manager.Confirm("unix(\"ls -d */\", path=\"./plans/[draft].md\")")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatalf("expected remembered literal permission to pass")
	}
}

func TestGlobEscapesLiteralCharacters(t *testing.T) {
	manager := NewConfirmationManager([]string{
		"unix(\"ls -d \\\\*/\", path=\"./plans/\\\\[draft\\\\].md\")",
	}, nil, nil, nil)

	allowed, err := manager.Confirm("unix(\"ls -d */\", path=\"./plans/[draft].md\")")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatalf("expected escaped glob metacharacters to match literally")
	}
}

func TestWildcardPermissionMatchesBroaderUnixCommand(t *testing.T) {
	manager := NewConfirmationManager([]string{
		"unix(\"ls -d */\")",
	}, nil, nil, nil)

	allowed, err := manager.Confirm("unix(\"ls -d ~/Desktop/*/\")")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatalf("expected wildcard unix permission to cover nested directory listing")
	}
}

func TestWildcardPermissionMatchesMoreSpecificUnixPath(t *testing.T) {
	manager := NewConfirmationManager([]string{
		"unix(\"ls -d ~/*/\")",
	}, nil, nil, nil)

	allowed, err := manager.Confirm("unix(\"ls -d ~/Desktop/*/\")")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatalf("expected broader unix wildcard permission to cover specific home path")
	}
}

func TestMultiPatternRemember(t *testing.T) {
	var remembered []string
	var rememberedAllow bool

	manager := NewConfirmationManager(nil, nil,
		func(action string) (confirmationDecision, error) {
			return confirmationDecision{
				allowed:  true,
				remember: true,
				patterns: []string{"unix(\"find *\")", "unix(\"sort *\")"},
			}, nil
		},
		func(actions []string, allow bool) error {
			remembered = actions
			rememberedAllow = allow
			return nil
		},
	)

	allowed, err := manager.Confirm("unix(\"find . -type f | sort\")")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected action to be allowed")
	}
	if len(remembered) != 2 || remembered[0] != "unix(\"find *\")" || remembered[1] != "unix(\"sort *\")" {
		t.Fatalf("expected multi-pattern remember, got: %v", remembered)
	}
	if !rememberedAllow {
		t.Fatal("expected remember to allow")
	}
}

func TestRememberedPatternMatchesWithinSameSession(t *testing.T) {
	promptCount := 0

	manager := NewConfirmationManager(nil, nil,
		func(action string) (confirmationDecision, error) {
			promptCount++
			return confirmationDecision{
				allowed:  true,
				remember: true,
				patterns: []string{tools.ToolNameUnix + `("find *")`},
			}, nil
		},
		func(actions []string, allow bool) error { return nil },
	)

	allowed, err := manager.Confirm(tools.ToolNameUnix + `("find . -type f")`)
	if err != nil {
		t.Fatalf("unexpected error on first confirm: %v", err)
	}
	if !allowed {
		t.Fatal("expected first action to be allowed")
	}
	if promptCount != 1 {
		t.Fatalf("expected exactly one prompt, got %d", promptCount)
	}

	allowed, err = manager.Confirm(tools.ToolNameUnix + `("find /tmp -name '*.go'")`)
	if err != nil {
		t.Fatalf("unexpected error on second confirm: %v", err)
	}
	if !allowed {
		t.Fatal("expected second find command to match the remembered pattern")
	}
	if promptCount != 1 {
		t.Fatalf("expected no additional prompt for matching pattern, got %d total", promptCount)
	}
}
