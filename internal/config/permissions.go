package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const localConfigFileName = ".terminal-agent.json"

type Permissions struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
	Ask   []string `json:"ask,omitempty"`
}

type PermissionRuleSet struct {
	Permissions Permissions
	Priority    int
	SourcePath  string
}

type PermissionStore struct {
	GlobalPath string
	LocalPaths []string
}

func LoadPermissionRuleSets(startDir string) ([]PermissionRuleSet, PermissionStore, error) {
	globalConfig, err := LoadConfig()
	if err != nil {
		return nil, PermissionStore{}, err
	}

	localPaths, err := FindLocalConfigPaths(startDir)
	if err != nil {
		return nil, PermissionStore{}, err
	}

	rules := []PermissionRuleSet{
		{
			Permissions: globalConfig.Permissions,
			Priority:    0,
			SourcePath:  getConfigPath(),
		},
	}

	for i, path := range localPaths {
		permissions, err := LoadLocalPermissions(path)
		if err != nil {
			return nil, PermissionStore{}, err
		}
		rules = append(rules, PermissionRuleSet{
			Permissions: permissions,
			Priority:    i + 1,
			SourcePath:  path,
		})
	}

	return rules, PermissionStore{
		GlobalPath: getConfigPath(),
		LocalPaths: localPaths,
	}, nil
}

func FindLocalConfigPaths(startDir string) ([]string, error) {
	if startDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		startDir = cwd
	}

	absolute, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}

	var matches []string
	current := absolute
	for {
		candidate := filepath.Join(current, localConfigFileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			matches = append(matches, candidate)
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	for i, j := 0, len(matches)-1; i < j; i, j = i+1, j-1 {
		matches[i], matches[j] = matches[j], matches[i]
	}

	return matches, nil
}

func LoadLocalPermissions(path string) (Permissions, error) {
	file, err := os.Open(path)
	if err != nil {
		return Permissions{}, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return Permissions{}, err
	}

	if len(strings.TrimSpace(string(content))) == 0 {
		return Permissions{}, nil
	}

	var payload struct {
		Permissions Permissions `json:"permissions"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		return Permissions{}, err
	}

	return payload.Permissions, nil
}

func RememberPermission(store PermissionStore, action string, allow bool) error {
	target := store.GlobalPath
	if len(store.LocalPaths) > 0 {
		target = store.LocalPaths[len(store.LocalPaths)-1]
	}

	if target == store.GlobalPath {
		config, err := LoadConfig()
		if err != nil {
			return err
		}
		config.Permissions = applyRememberedPermission(config.Permissions, action, allow)
		return SaveConfig(config)
	}

	return updateLocalPermissionsFile(target, action, allow)
}

func applyRememberedPermission(permissions Permissions, action string, allow bool) Permissions {
	if allow {
		permissions.Allow = ensurePermission(permissions.Allow, action)
		permissions.Deny = removePermission(permissions.Deny, action)
		return permissions
	}

	permissions.Deny = ensurePermission(permissions.Deny, action)
	permissions.Allow = removePermission(permissions.Allow, action)
	return permissions
}

func ensurePermission(values []string, action string) []string {
	for _, value := range values {
		if value == action {
			return values
		}
	}
	return append(values, action)
}

func removePermission(values []string, action string) []string {
	if len(values) == 0 {
		return values
	}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value != action {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func updateLocalPermissionsFile(path string, action string, allow bool) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	payload := map[string]any{}
	if len(strings.TrimSpace(string(content))) > 0 {
		if err := json.Unmarshal(content, &payload); err != nil {
			return err
		}
	}

	permissions := Permissions{}
	if existing, ok := payload["permissions"]; ok {
		encoded, err := json.Marshal(existing)
		if err != nil {
			return fmt.Errorf("failed to parse permissions in %s: %w", path, err)
		}
		if err := json.Unmarshal(encoded, &permissions); err != nil {
			return fmt.Errorf("failed to decode permissions in %s: %w", path, err)
		}
	}

	permissions = applyRememberedPermission(permissions, action, allow)
	payload["permissions"] = permissions

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(payload)
}
