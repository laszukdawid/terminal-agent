package agent

import "testing"

func TestIsReadOnlyUnixCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{name: "ls", command: "ls -la", want: true},
		{name: "find pipeline", command: "find . -type f | sort | head -20", want: true},
		{name: "grep wc pipeline", command: `ls -la | grep "go$" | wc -l`, want: true},
		{name: "quoted pipe", command: `grep "a|b" file.txt | wc -l`, want: true},
		{name: "double quoted arg", command: `ls "my dir"`, want: true},
		{name: "env display", command: "env", want: true},
		{name: "env variable display", command: "env FOO=bar", want: true},
		{name: "path helpers", command: "basename ./foo/bar | dirname ./foo/bar | realpath .", want: true},
		{name: "checksum and lookup", command: "sha256sum go.mod | cut -d ' ' -f 1", want: true},
		{name: "diff which tree", command: "diff go.mod go.mod | which go | tree .", want: true},
		{name: "find delete", command: "find . -delete", want: false},
		{name: "find exec", command: `find . -exec rm {} \;`, want: false},
		{name: "redirection", command: "ls > out.txt", want: false},
		{name: "negated", command: "! ls", want: false},
		{name: "background", command: "ls &", want: false},
		{name: "and operator", command: "ls && rm file", want: false},
		{name: "semicolon", command: "ls; rm file", want: false},
		{name: "command substitution", command: "echo $(rm file)", want: false},
		{name: "process substitution", command: "cat <(rm file)", want: false},
		{name: "tee writes", command: "cat file | tee out.txt", want: false},
		{name: "env with command", command: "env rm file", want: false},
		{name: "variable assignment", command: "FOO=bar ls", want: false},
		{name: "dollar single quoted", command: "ls $'my dir'", want: false},
		{name: "absolute path", command: "/bin/ls", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isReadOnlyUnixCommand(tt.command); got != tt.want {
				t.Fatalf("isReadOnlyUnixCommand(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}
