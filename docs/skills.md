# Skills

Skills are reusable task instructions stored as local Markdown files. They are available to `agent task` and can be loaded during execution with the built-in `skill` tool.

## Locations

Terminal Agent discovers skills in this priority order:

1. Project-local: `.terminal-agent/skills/<skill-name>/SKILL.md`
2. Configured paths from `skills.paths`
3. Global: `~/.config/terminal-agent/skills/<skill-name>/SKILL.md`

If multiple skills use the same name, the higher-priority skill wins.

## Format

Each skill is a `SKILL.md` file with YAML frontmatter:

```md
---
name: code-reviewer
description: Use when reviewing diffs, pull requests, or code quality.
---

# Code Reviewer

Read the relevant files and diffs before giving feedback. Prioritize correctness,
security, regressions, and missing tests.
```

Skill names must use lowercase letters, numbers, and hyphens.

## Commands

Create a project-local skill:

```sh
agent skill create code-reviewer \
  --description "Use when reviewing diffs, pull requests, or code quality." \
  --instructions "Read relevant files first. Prioritize bugs, regressions, security, and missing tests."
```

Create a global skill:

```sh
agent skill create code-reviewer --global \
  --description "Use when reviewing diffs, pull requests, or code quality."
```

List discovered skills:

```sh
agent skill list
```

Show a skill:

```sh
agent skill show code-reviewer
```

## Task Behavior

`agent task` injects only a compact index of available skill names and descriptions. When a skill is relevant, the agent loads the full instructions with the `skill` tool.

Skill discovery refreshes each model turn. If a task creates a new project-local skill, it can become available later in the same task after the next refresh.

Skills are prompt instructions, not permissions or executable plugins. Existing tool confirmation and filesystem scope rules still apply.
