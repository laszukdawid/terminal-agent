package tools

import "testing"

func TestValidateResCodeAllowsPythonExecution(t *testing.T) {
	cases := []string{
		"python3 -c 'print(1)'",
		"python -c 'print(1)'",
		"uv run python -c 'print(1)'",
	}

	for _, cmd := range cases {
		if err := validateResCode(cmd); err != nil {
			t.Fatalf("expected %q to be allowed, got error: %v", cmd, err)
		}
	}
}
