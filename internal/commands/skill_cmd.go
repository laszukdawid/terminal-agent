package commands

import (
	"fmt"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/config"
	internalskills "github.com/laszukdawid/terminal-agent/internal/skills"
	"github.com/spf13/cobra"
)

func NewSkillCommand(cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "List, create, and show task skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(skillListCommand(cfg), skillShowCommand(cfg), skillCreateCommand(cfg))
	return cmd
}

func skillListCommand(cfg config.Config) *cobra.Command {
	var root string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			root = resolveSkillRoot(root, cfg)
			skills, err := internalskills.Discover(root, cfg)
			if err != nil {
				return err
			}
			if len(skills) == 0 {
				cmd.Println("No skills found.")
				return nil
			}
			for _, skill := range skills {
				cmd.Printf("%s\t%s\t%s\n", skill.Name, skill.Source, skill.Description)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&root, "root", "", "Project root used to discover project-local skills")
	return cmd
}

func skillShowCommand(cfg config.Config) *cobra.Command {
	var root string
	cmd := &cobra.Command{
		Use:   "show [skill-name]",
		Short: "Show a skill file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root = resolveSkillRoot(root, cfg)
			content, err := internalskills.Show(root, cfg, args[0])
			if err != nil {
				return err
			}
			cmd.Println(content)
			return nil
		},
	}
	cmd.Flags().StringVar(&root, "root", "", "Project root used to discover project-local skills")
	return cmd
}

func skillCreateCommand(cfg config.Config) *cobra.Command {
	var root string
	var global bool
	var description string
	var instructions string
	var overwrite bool
	cmd := &cobra.Command{
		Use:   "create [skill-name]",
		Short: "Create a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := internalskills.NormalizeName(args[0])
			if !internalskills.ValidName(name) {
				return fmt.Errorf("invalid skill name %q: use kebab-case letters, numbers, and hyphens", args[0])
			}
			if strings.TrimSpace(description) == "" {
				return fmt.Errorf("--description is required")
			}
			dir := internalskills.GlobalDir()
			if !global {
				root = resolveSkillRoot(root, cfg)
				dir = internalskills.ProjectDir(root)
			}
			skill, err := internalskills.Create(dir, name, description, instructions, overwrite)
			if err != nil {
				return err
			}
			cmd.Printf("Created skill %s at %s\n", skill.Name, skill.Path)
			return nil
		},
	}
	cmd.Flags().StringVar(&root, "root", "", "Project root for project-local skill creation")
	cmd.Flags().BoolVar(&global, "global", false, "Create the skill in the global skills directory")
	cmd.Flags().StringVar(&description, "description", "", "Skill description used in the available skills index")
	cmd.Flags().StringVar(&instructions, "instructions", "", "Initial skill instructions")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite an existing skill")
	return cmd
}

func resolveSkillRoot(root string, cfg config.Config) string {
	root = strings.TrimSpace(root)
	if root != "" {
		return root
	}
	if cfg != nil {
		return cfg.GetWorkingDir()
	}
	return "."
}
