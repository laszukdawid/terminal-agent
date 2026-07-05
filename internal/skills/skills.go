package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	SkillFileName        = "SKILL.md"
	ProjectSkillsSubdir  = ".terminal-agent/skills"
	GlobalSkillsSubdir   = ".config/terminal-agent/skills"
	SourceProject        = "project"
	SourceConfiguredPath = "configured"
	SourceGlobal         = "global"
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var quotedPhrasePattern = regexp.MustCompile(`"([^"]+)"|'([^']+)'`)

var (
	ErrInvalidName = errors.New("invalid skill name")
	ErrNotFound    = errors.New("skill not found")
)

type Config interface {
	GetSkillsEnabled() bool
	GetSkillPaths() []string
}

type Skill struct {
	Name        string
	Description string
	Path        string
	Source      string
	Content     string
}

type Source struct {
	Path   string
	Kind   string
	Rank   int
	Exists bool
}

func ValidName(name string) bool {
	return skillNamePattern.MatchString(strings.TrimSpace(name))
}

func NormalizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "-")
	name = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(name, "-")
	name = regexp.MustCompile(`-+`).ReplaceAllString(name, "-")
	return strings.Trim(name, "-")
}

func ProjectDir(root string) string {
	return filepath.Join(root, ProjectSkillsSubdir)
}

func GlobalDir() string {
	return filepath.Join(os.Getenv("HOME"), GlobalSkillsSubdir)
}

func ResolveSources(root string, cfg any) []Source {
	sources := []Source{{Path: ProjectDir(root), Kind: SourceProject, Rank: 0}}
	if skillConfig, ok := cfg.(Config); ok {
		for _, path := range skillConfig.GetSkillPaths() {
			trimmed := strings.TrimSpace(path)
			if trimmed == "" {
				continue
			}
			sources = append(sources, Source{Path: expandHome(trimmed), Kind: SourceConfiguredPath, Rank: len(sources)})
		}
	}
	sources = append(sources, Source{Path: GlobalDir(), Kind: SourceGlobal, Rank: len(sources)})
	for i := range sources {
		info, err := os.Stat(sources[i].Path)
		sources[i].Exists = err == nil && info.IsDir()
	}
	return sources
}

func Discover(root string, cfg any) ([]Skill, error) {
	if skillConfig, ok := cfg.(Config); ok && !skillConfig.GetSkillsEnabled() {
		return nil, nil
	}

	byName := map[string]Skill{}
	for _, source := range ResolveSources(root, cfg) {
		if !source.Exists {
			continue
		}
		entries, err := os.ReadDir(source.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to read skills directory %s: %w", source.Path, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillPath := filepath.Join(source.Path, entry.Name(), SkillFileName)
			skill, err := Read(skillPath)
			if err != nil || skill.Name == "" {
				continue
			}
			if _, exists := byName[skill.Name]; exists {
				continue
			}
			skill.Source = source.Kind
			byName[skill.Name] = skill
		}
	}

	out := make([]Skill, 0, len(byName))
	for _, skill := range byName {
		out = append(out, skill)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func Load(root string, cfg any, name string) (Skill, error) {
	if !ValidName(name) {
		return Skill{}, ErrInvalidName
	}
	skills, err := Discover(root, cfg)
	if err != nil {
		return Skill{}, err
	}
	for _, skill := range skills {
		if skill.Name == name {
			return Read(skill.Path)
		}
	}
	return Skill{}, ErrNotFound
}

func Show(root string, cfg any, name string) (string, error) {
	skill, err := Load(root, cfg, name)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(skill.Path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func Read(path string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	content := string(data)
	meta, body := parseFrontmatter(content)
	name := strings.TrimSpace(meta["name"])
	description := strings.TrimSpace(meta["description"])
	if !ValidName(name) {
		return Skill{}, ErrInvalidName
	}
	if description == "" {
		return Skill{}, fmt.Errorf("skill %s missing description", name)
	}
	return Skill{Name: name, Description: description, Path: path, Content: strings.TrimSpace(body)}, nil
}

func FormatIndex(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Available skills (load full instructions with the skill tool when relevant):\n")
	for _, skill := range skills {
		fmt.Fprintf(&b, "- %s: %s\n", skill.Name, skill.Description)
	}
	return strings.TrimRight(b.String(), "\n")
}

func TriggeredBy(skill Skill, prompt string) bool {
	text := normalizeTriggerText(prompt)
	if text == "" {
		return false
	}
	for _, trigger := range skillTriggers(skill) {
		if strings.Contains(text, trigger) {
			return true
		}
	}
	return false
}

func Triggered(skills []Skill, prompt string) []Skill {
	triggered := make([]Skill, 0, len(skills))
	for _, skill := range skills {
		if TriggeredBy(skill, prompt) {
			triggered = append(triggered, skill)
		}
	}
	return triggered
}

func FormatLoaded(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Triggered skill instructions (MUST follow these instructions for this task):\n")
	for i, skill := range skills {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "<skill name=%q>\n%s\n</skill>", skill.Name, strings.TrimSpace(skill.Content))
	}
	return b.String()
}

func skillTriggers(skill Skill) []string {
	seen := map[string]struct{}{}
	add := func(value string) {
		value = normalizeTriggerText(value)
		if value == "" {
			return
		}
		seen[value] = struct{}{}
	}
	add(skill.Name)
	add(strings.ReplaceAll(skill.Name, "-", " "))
	for _, match := range quotedPhrasePattern.FindAllStringSubmatch(skill.Description, -1) {
		if match[1] != "" {
			add(match[1])
		}
		if match[2] != "" {
			add(match[2])
		}
	}
	out := make([]string, 0, len(seen))
	for trigger := range seen {
		out = append(out, trigger)
	}
	sort.Strings(out)
	return out
}

func normalizeTriggerText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	value = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(value, " ")
	return strings.Join(strings.Fields(value), " ")
}

func NewSkillContent(name string, description string, instructions string) (string, error) {
	name = NormalizeName(name)
	if !ValidName(name) {
		return "", ErrInvalidName
	}
	description = strings.TrimSpace(description)
	if description == "" {
		return "", fmt.Errorf("description is required")
	}
	instructions = strings.TrimSpace(instructions)
	if instructions == "" {
		instructions = "Describe when to use this skill and the workflow the agent should follow."
	}
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n# %s\n\n%s\n", name, description, titleFromName(name), instructions), nil
}

func Create(dir string, name string, description string, instructions string, overwrite bool) (Skill, error) {
	name = NormalizeName(name)
	content, err := NewSkillContent(name, description, instructions)
	if err != nil {
		return Skill{}, err
	}
	path := filepath.Join(dir, name, SkillFileName)
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return Skill{}, fmt.Errorf("skill already exists: %s", name)
		} else if !os.IsNotExist(err) {
			return Skill{}, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Skill{}, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return Skill{}, err
	}
	return Read(path)
}

func parseFrontmatter(content string) (map[string]string, string) {
	meta := map[string]string{}
	trimmed := strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(trimmed, "---\n") {
		return meta, content
	}
	rest := strings.TrimPrefix(trimmed, "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return meta, content
	}
	frontmatter := rest[:end]
	body := strings.TrimPrefix(rest[end:], "\n---")
	body = strings.TrimPrefix(body, "\r\n")
	body = strings.TrimPrefix(body, "\n")
	for _, line := range strings.Split(frontmatter, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		meta[key] = value
	}
	return meta, body
}

func expandHome(path string) string {
	if path == "~" {
		return os.Getenv("HOME")
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(os.Getenv("HOME"), path[2:])
	}
	return path
}

func titleFromName(name string) string {
	parts := strings.Split(name, "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
