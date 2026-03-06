package agent

import "testing"

func TestBuildActionString(t *testing.T) {
	action := BuildActionString("unix", map[string]any{
		"command": "aws login sso",
		"flag":    "value",
	})

	if action != "unix(\"aws login sso\", flag=\"value\")" {
		t.Fatalf("unexpected action string: %s", action)
	}
}

func TestConfirmationAllowsExactAndPrefix(t *testing.T) {
	manager := NewConfirmationManager([]string{
		"unix(\"aws login\")",
		"unix(\"cat .*\")",
	}, nil, nil)

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
		t.Fatalf("expected prefix allow to pass")
	}
}

func TestAllowKeysRegex(t *testing.T) {
	manager := NewConfirmationManager([]string{
		"unix(\"aws login\", allowKeys=[\"region\", \"profile\", \"read.*\"])",
		"unix(\"aws login\", region=\"us-.*\")",
	}, nil, nil)

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
