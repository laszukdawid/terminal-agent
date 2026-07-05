package commands

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/config"
	internalskills "github.com/laszukdawid/terminal-agent/internal/skills"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillCreateListShowCommands(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	cfg := config.NewDefaultConfig()
	cfg.SetWorkingDir(root)

	cmd := NewSkillCommand(cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"create", "Code Reviewer", "--description", "Review code", "--instructions", "Read diffs carefully."})
	require.NoError(t, cmd.Execute())
	assert.FileExists(t, filepath.Join(internalskills.ProjectDir(root), "code-reviewer", internalskills.SkillFileName))

	out.Reset()
	cmd = NewSkillCommand(cfg)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"list"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "code-reviewer")
	assert.Contains(t, out.String(), "Review code")

	out.Reset()
	cmd = NewSkillCommand(cfg)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"show", "code-reviewer"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "name: code-reviewer")
	assert.Contains(t, out.String(), "description: Review code")
	assert.Contains(t, out.String(), "Read diffs carefully.")
}
