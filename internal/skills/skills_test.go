package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testConfig struct {
	enabled bool
	paths   []string
}

func (c testConfig) GetSkillsEnabled() bool  { return c.enabled }
func (c testConfig) GetSkillPaths() []string { return c.paths }

func TestDiscoverPrefersProjectSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	writeSkill(t, filepath.Join(GlobalDir(), "reviewer", SkillFileName), "reviewer", "global desc", "global body")
	writeSkill(t, filepath.Join(ProjectDir(root), "reviewer", SkillFileName), "reviewer", "project desc", "project body")

	skills, err := Discover(root, testConfig{enabled: true})
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "reviewer", skills[0].Name)
	assert.Equal(t, "project desc", skills[0].Description)
	assert.Equal(t, SourceProject, skills[0].Source)
}

func TestLoadReadsHighestPrioritySkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	writeSkill(t, filepath.Join(GlobalDir(), "release", SkillFileName), "release", "global", "global body")
	writeSkill(t, filepath.Join(ProjectDir(root), "release", SkillFileName), "release", "project", "project body")

	skill, err := Load(root, testConfig{enabled: true}, "release")
	require.NoError(t, err)
	assert.Equal(t, "project body", skill.Content)
}

func TestCreateNormalizesName(t *testing.T) {
	dir := t.TempDir()
	skill, err := Create(dir, "Code Reviewer", "Review code", "Use review workflow.", false)
	require.NoError(t, err)
	assert.Equal(t, "code-reviewer", skill.Name)
	assert.FileExists(t, filepath.Join(dir, "code-reviewer", SkillFileName))
}

func TestFormatIndex(t *testing.T) {
	index := FormatIndex([]Skill{{Name: "code-reviewer", Description: "Review code"}})
	assert.Contains(t, index, "- code-reviewer: Review code")
}

func TestTriggeredMatchesNameAndQuotedDescriptionPhrase(t *testing.T) {
	skills := []Skill{
		{Name: "abra-kadabra", Description: `Use when the prompt contains "abra kadabra".`},
		{Name: "code-reviewer", Description: "Review code"},
	}

	triggered := Triggered(skills, "show me the time abra kadabra")
	require.Len(t, triggered, 1)
	assert.Equal(t, "abra-kadabra", triggered[0].Name)
}

func TestFormatLoaded(t *testing.T) {
	loaded := FormatLoaded([]Skill{{Name: "abra-kadabra", Content: "Speak like a wizard."}})
	assert.Contains(t, loaded, `Triggered skill instructions`)
	assert.Contains(t, loaded, `<skill name="abra-kadabra">`)
	assert.Contains(t, loaded, "Speak like a wizard.")
}

func writeSkill(t *testing.T, path string, name string, description string, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
