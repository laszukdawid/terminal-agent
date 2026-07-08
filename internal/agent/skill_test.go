package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/config"
	internalskills "github.com/laszukdawid/terminal-agent/internal/skills"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPromptIncludesAvailableSkillsSection(t *testing.T) {
	state := &TaskState{
		OriginalQuery: "review code",
		Phase:         TaskPhaseRunning,
		MaxTurns:      5,
		MaxIterations: 5,
		Dirs:          TaskDirs{RootDir: "/repo", CurrentDir: "/repo"},
		Skills:        []internalskills.Skill{{Name: "code-reviewer", Description: "Review code"}},
		LoadedSkills:  []internalskills.Skill{{Name: "code-reviewer", Content: "Read diffs carefully."}},
	}

	prompt, err := buildTaskPrompt(state)
	require.NoError(t, err)
	assert.Contains(t, prompt, "Current working directory: /repo")
	assert.Contains(t, prompt, "Available skills")
	assert.Contains(t, prompt, "code-reviewer")
	assert.Contains(t, prompt, "Triggered skill instructions")
	assert.Contains(t, prompt, "Read diffs carefully")
}

func TestSkillToolLoadsSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	path := filepath.Join(internalskills.ProjectDir(root), "code-reviewer", internalskills.SkillFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("---\nname: code-reviewer\ndescription: Review code\n---\n\nRead diffs carefully.\n"), 0o644))

	run := &taskExecutionState{state: &TaskState{Dirs: TaskDirs{RootDir: root}}, config: config.NewDefaultConfig()}
	tool := NewSkillTool(run)
	out, err := tool.RunSchema(map[string]any{"name": "code-reviewer"})
	require.NoError(t, err)
	assert.Contains(t, out, "Read diffs carefully.")
}
