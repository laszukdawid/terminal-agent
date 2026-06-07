package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsReadOnlyUnixCommandInDirsWithoutPathContext(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{name: "ls", command: "ls -la", want: true},
		{name: "find pipeline", command: "find . -type f | sort | head -20", want: true},
		{name: "grep pipeline true", command: "grep -RIl --exclude-dir=.git --exclude-dir=vendor --exclude=agent --exclude=agent-gui -e 'package ' /home/dawid/projects/terminal-agent | true", want: true},
		{name: "grep wc pipeline", command: `ls -la | grep "go$" | wc -l`, want: true},
		{name: "quoted pipe", command: `grep "a|b" file.txt | wc -l`, want: true},
		{name: "double quoted arg", command: `ls "my dir"`, want: true},
		{name: "env display", command: "env", want: true},
		{name: "env variable display", command: "env FOO=bar", want: true},
		{name: "echo", command: "echo hello", want: true},
		{name: "colon no-op", command: ":", want: true},
		{name: "colon static args", command: ": ignored values", want: true},
		{name: "false", command: "false", want: true},
		{name: "printf", command: "printf '%s\\n' hello", want: true},
		{name: "test", command: "test -d .", want: true},
		{name: "bracket test", command: "[ -d . ]", want: true},
		{name: "true", command: "true", want: true},
		{name: "awk field separator", command: `awk -F/ '{print $1}' file.txt`, want: true},
		{name: "awk variable", command: `awk -v label=name '{print label, $0}' file.txt`, want: true},
		{name: "path helpers", command: "basename ./foo/bar | dirname ./foo/bar | realpath .", want: true},
		{name: "checksum and lookup", command: "sha256sum go.mod | cut -d ' ' -f 1", want: true},
		{name: "diff which tree", command: "diff go.mod go.mod | which go | tree .", want: true},
		{name: "semicolon safe commands", command: "pwd; ls", want: true},
		{name: "find delete", command: "find . -delete", want: false},
		{name: "find exec", command: `find . -exec rm {} \;`, want: false},
		{name: "redirection", command: "ls > out.txt", want: false},
		{name: "negated", command: "! ls", want: false},
		{name: "background", command: "ls &", want: false},
		{name: "and operator", command: "ls && rm file", want: false},
		{name: "semicolon", command: "ls; rm file", want: false},
		{name: "or operator", command: "ls || pwd", want: false},
		{name: "while loop", command: "while pwd; do echo ok; done", want: false},
		{name: "command substitution", command: "echo $(rm file)", want: false},
		{name: "process substitution", command: "cat <(rm file)", want: false},
		{name: "tee writes", command: "cat file | tee out.txt", want: false},
		{name: "env with command", command: "env rm file", want: false},
		{name: "false with ignored arg", command: "false ignored", want: false},
		{name: "bracket test missing close", command: "[ -d .", want: false},
		{name: "printf command substitution", command: "printf '%s\\n' $(rm file)", want: false},
		{name: "true with ignored arg", command: "true ignored", want: false},
		{name: "variable assignment", command: "FOO=bar ls", want: false},
		{name: "dollar single quoted", command: "ls $'my dir'", want: false},
		{name: "absolute path", command: "/bin/ls", want: false},
		{name: "awk system", command: `awk 'BEGIN{system("rm file")}'`, want: false},
		{name: "awk system with spacing", command: `awk 'BEGIN{system ("rm file")}'`, want: false},
		{name: "awk output redirection", command: `awk '{print $0 > "out.txt"}'`, want: false},
		{name: "awk external script", command: `awk -f script.awk input.txt`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isReadOnlyUnixCommandInDirs(tt.command, TaskDirs{}); got != tt.want {
				t.Fatalf("isReadOnlyUnixCommandInDirs(%q, empty dirs) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

func TestIsReadOnlyUnixCommandInDirs(t *testing.T) {
	rootDir := t.TempDir()
	assetsDir := filepath.Join(rootDir, "assets")
	nestedDir := filepath.Join(rootDir, "nested")
	outsideDir := t.TempDir()
	linkDir := filepath.Join(rootDir, "outside-link")

	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, linkDir); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{name: "cd root and find", command: "cd " + rootDir + " && find assets -maxdepth 1 -type f -printf '%f'", want: true},
		{name: "cd root semicolon find", command: "cd " + rootDir + "; find assets -maxdepth 1 -type f -printf '%f'", want: true},
		{name: "cd rg awk sort pipeline", command: "cd " + rootDir + ` && rg -l -w 'package' . | awk 'BEGIN{FS="/"} {n=split($0,a,"/"); f=a[n]; print length(f) "\t" $0}' | sort -n -k1,1 -k2,2`, want: true},
		{name: "cd relative directory", command: "cd nested; pwd", want: true},
		{name: "cd back to root", command: "cd nested; cd ..; pwd", want: true},
		{name: "cd outside root", command: "cd " + outsideDir + "; find . -type f", want: false},
		{name: "cd symlink outside root", command: "cd " + linkDir + "; find . -type f", want: false},
		{name: "cd root then unsafe find", command: "cd " + rootDir + "; find . -delete", want: false},
		{name: "cd in pipeline", command: "cd " + rootDir + " | pwd", want: false},
		{name: "static for loop", command: "for i in 1 2 3; do echo \"$i\"; pwd; done", want: true},
		{name: "for loop without static items", command: "for i; do echo \"$i\"; done", want: false},
		{name: "for loop repeats directory changes", command: "cd nested; for i in 1 2; do cd ..; done; pwd", want: false},
		{name: "for loop command substitution item", command: "for i in $(ls); do echo \"$i\"; done", want: false},
		{name: "for loop unsafe body", command: "for i in 1 2 3; do rm file; done", want: false},
		{name: "for loop unsafe variable use", command: "for i in 1 2 3; do find \"$i\" -type f; done", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirs := TaskDirs{RootDir: rootDir, CurrentDir: rootDir}
			if got := isReadOnlyUnixCommandInDirs(tt.command, dirs); got != tt.want {
				t.Fatalf("isReadOnlyUnixCommandInDirs(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}
