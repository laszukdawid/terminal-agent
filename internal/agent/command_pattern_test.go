package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTokenizeCommand(t *testing.T) {
	t.Run("simple command with flags", func(t *testing.T) {
		groups := TokenizeCommand("find . -maxdepth 2 -type d")
		assert.Equal(t, []TokenGroup{
			{Raw: "find", Kind: TokenCommand},
			{Raw: ".", Kind: TokenArg},
			{Raw: "-maxdepth 2", Kind: TokenFlagPair},
			{Raw: "-type d", Kind: TokenFlagPair},
		}, groups)
	})

	t.Run("combined short flag with following arg", func(t *testing.T) {
		// -la is followed by a non-flag token, so the heuristic treats it as a flag pair.
		// This is a best-effort grouping since we can't know the command's syntax.
		groups := TokenizeCommand("ls -la /tmp")
		assert.Equal(t, []TokenGroup{
			{Raw: "ls", Kind: TokenCommand},
			{Raw: "-la /tmp", Kind: TokenFlagPair},
		}, groups)
	})

	t.Run("boolean flag at end", func(t *testing.T) {
		groups := TokenizeCommand("ls /tmp -la")
		assert.Equal(t, []TokenGroup{
			{Raw: "ls", Kind: TokenCommand},
			{Raw: "/tmp", Kind: TokenArg},
			{Raw: "-la", Kind: TokenBooleanFlag},
		}, groups)
	})

	t.Run("boolean flag followed by another flag", func(t *testing.T) {
		groups := TokenizeCommand("ls -l -a /tmp")
		assert.Equal(t, []TokenGroup{
			{Raw: "ls", Kind: TokenCommand},
			{Raw: "-l", Kind: TokenBooleanFlag},
			{Raw: "-a /tmp", Kind: TokenFlagPair},
		}, groups)
	})

	t.Run("piped commands", func(t *testing.T) {
		groups := TokenizeCommand("find . -type f | sort | head -20")
		assert.Equal(t, []TokenGroup{
			{Raw: "find", Kind: TokenCommand},
			{Raw: ".", Kind: TokenArg},
			{Raw: "-type f", Kind: TokenFlagPair},
			{Raw: "| sort", Kind: TokenPipeCommand},
			{Raw: "| head", Kind: TokenPipeCommand},
			{Raw: "-20", Kind: TokenArg},
		}, groups)
	})

	t.Run("command with quoted argument", func(t *testing.T) {
		groups := TokenizeCommand("sed 's#^./##'")
		assert.Equal(t, []TokenGroup{
			{Raw: "sed", Kind: TokenCommand},
			{Raw: "'s#^./##'", Kind: TokenArg},
		}, groups)
	})

	t.Run("pipe in quotes is not split", func(t *testing.T) {
		groups := TokenizeCommand(`grep "foo|bar" file.txt`)
		assert.Equal(t, []TokenGroup{
			{Raw: "grep", Kind: TokenCommand},
			{Raw: `"foo|bar"`, Kind: TokenArg},
			{Raw: "file.txt", Kind: TokenArg},
		}, groups)
	})

	t.Run("single command no args", func(t *testing.T) {
		groups := TokenizeCommand("ls")
		assert.Equal(t, []TokenGroup{
			{Raw: "ls", Kind: TokenCommand},
		}, groups)
	})

	t.Run("long flag pair", func(t *testing.T) {
		groups := TokenizeCommand("git log --oneline --max-count 5")
		assert.Equal(t, []TokenGroup{
			{Raw: "git", Kind: TokenCommand},
			{Raw: "log", Kind: TokenArg},
			{Raw: "--oneline", Kind: TokenBooleanFlag},
			{Raw: "--max-count 5", Kind: TokenFlagPair},
		}, groups)
	})

	t.Run("flag at end treated as boolean", func(t *testing.T) {
		groups := TokenizeCommand("ls -r")
		assert.Equal(t, []TokenGroup{
			{Raw: "ls", Kind: TokenCommand},
			{Raw: "-r", Kind: TokenBooleanFlag},
		}, groups)
	})

	t.Run("complex piped command", func(t *testing.T) {
		groups := TokenizeCommand("find . -maxdepth 2 -type d | sort | sed 's#^./##' | head -80")
		assert.Equal(t, []TokenGroup{
			{Raw: "find", Kind: TokenCommand},
			{Raw: ".", Kind: TokenArg},
			{Raw: "-maxdepth 2", Kind: TokenFlagPair},
			{Raw: "-type d", Kind: TokenFlagPair},
			{Raw: "| sort", Kind: TokenPipeCommand},
			{Raw: "| sed", Kind: TokenPipeCommand},
			{Raw: "'s#^./##'", Kind: TokenArg},
			{Raw: "| head", Kind: TokenPipeCommand},
			{Raw: "-80", Kind: TokenArg},
		}, groups)
	})

	t.Run("subshell pipe not split", func(t *testing.T) {
		groups := TokenizeCommand("echo $(cat file | sort)")
		assert.Equal(t, []TokenGroup{
			{Raw: "echo", Kind: TokenCommand},
			{Raw: "$(cat file | sort)", Kind: TokenArg},
		}, groups)
	})
}

func TestGeneratePatternLevels(t *testing.T) {
	t.Run("simple command with flags", func(t *testing.T) {
		groups := TokenizeCommand("find . -maxdepth 2 -type d")
		levels := GeneratePatternLevels(groups)
		assert.Equal(t, []string{
			"find . -maxdepth 2 -type d",
			"find . -maxdepth 2 *",
			"find . *",
			"find *",
		}, levels)
	})

	t.Run("single command", func(t *testing.T) {
		groups := TokenizeCommand("ls")
		levels := GeneratePatternLevels(groups)
		assert.Equal(t, []string{"ls"}, levels)
	})

	t.Run("piped commands", func(t *testing.T) {
		groups := TokenizeCommand("find . -type f | sort | head -20")
		levels := GeneratePatternLevels(groups)
		assert.Equal(t, []string{
			"find . -type f | sort | head -20",
			"find . -type f | sort | head *",
			"find . -type f | sort *",
			"find . -type f *",
			"find . *",
			"find *",
		}, levels)
	})

	t.Run("empty input", func(t *testing.T) {
		levels := GeneratePatternLevels(nil)
		assert.Equal(t, []string{"*"}, levels)
	})
}


func TestSplitPipeSegments(t *testing.T) {
	t.Run("no pipes", func(t *testing.T) {
		segments := splitPipeSegments("find . -type f")
		assert.Equal(t, []string{"find . -type f"}, segments)
	})

	t.Run("simple pipes", func(t *testing.T) {
		segments := splitPipeSegments("cat file | sort | uniq")
		assert.Equal(t, []string{"cat file ", " sort ", " uniq"}, segments)
	})

	t.Run("pipe inside single quotes", func(t *testing.T) {
		segments := splitPipeSegments("grep 'a|b' file.txt")
		assert.Equal(t, []string{"grep 'a|b' file.txt"}, segments)
	})

	t.Run("pipe inside double quotes", func(t *testing.T) {
		segments := splitPipeSegments(`grep "a|b" file.txt`)
		assert.Equal(t, []string{`grep "a|b" file.txt`}, segments)
	})

	t.Run("pipe inside subshell", func(t *testing.T) {
		segments := splitPipeSegments("echo $(cat file | sort)")
		assert.Equal(t, []string{"echo $(cat file | sort)"}, segments)
	})
}

func TestTokenizeRespectingQuotes(t *testing.T) {
	t.Run("simple tokens", func(t *testing.T) {
		tokens := tokenizeRespectingQuotes("find . -type f")
		assert.Equal(t, []string{"find", ".", "-type", "f"}, tokens)
	})

	t.Run("single quoted arg", func(t *testing.T) {
		tokens := tokenizeRespectingQuotes("sed 's#^./##'")
		assert.Equal(t, []string{"sed", "'s#^./##'"}, tokens)
	})

	t.Run("double quoted arg with spaces", func(t *testing.T) {
		tokens := tokenizeRespectingQuotes(`echo "hello world"`)
		assert.Equal(t, []string{"echo", `"hello world"`}, tokens)
	})

	t.Run("empty input", func(t *testing.T) {
		tokens := tokenizeRespectingQuotes("")
		assert.Empty(t, tokens)
	})
}

func TestIsFlag(t *testing.T) {
	assert.True(t, isFlag("-v"))
	assert.True(t, isFlag("-type"))
	assert.True(t, isFlag("--oneline"))
	assert.True(t, isFlag("--max-count"))
	assert.False(t, isFlag("-"))
	assert.False(t, isFlag("-80"))
	assert.False(t, isFlag("foo"))
	assert.False(t, isFlag(""))
}
