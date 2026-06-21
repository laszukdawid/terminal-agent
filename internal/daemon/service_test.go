package daemon

import (
	"os"
	"strings"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/routines"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingRunner struct {
	calls [][]string
}

func (r *recordingRunner) run(name string, args ...string) error {
	r.calls = append(r.calls, append([]string{name}, args...))
	return nil
}

func (r *recordingRunner) sawCommand(substrings ...string) bool {
	for _, call := range r.calls {
		joined := strings.Join(call, " ")
		all := true
		for _, s := range substrings {
			if !strings.Contains(joined, s) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

func TestServiceManagerInstallUninstall(t *testing.T) {
	t.Setenv(routines.DataDirEnv, t.TempDir()) // daemon.log path on macOS

	tests := []struct {
		name        string
		goos        string
		unitPath    func(home string) string
		wantContent string
		installCmd  []string
		uninstall   []string
	}{
		{
			name:        "darwin launchd",
			goos:        "darwin",
			unitPath:    func(home string) string { return home + "/Library/LaunchAgents/" + launchdLabel + ".plist" },
			wantContent: "<string>daemon</string>",
			installCmd:  []string{"launchctl", "bootstrap"},
			uninstall:   []string{"launchctl", "bootout"},
		},
		{
			name:        "linux systemd",
			goos:        "linux",
			unitPath:    func(home string) string { return home + "/.config/systemd/user/" + systemdUnit },
			wantContent: "ExecStart=/usr/local/bin/agent daemon start",
			installCmd:  []string{"systemctl", "--user", "enable"},
			uninstall:   []string{"systemctl", "--user", "disable"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			rec := &recordingRunner{}
			m := &ServiceManager{goos: tt.goos, homeDir: home, exePath: "/usr/local/bin/agent", uid: 501, run: rec.run}

			require.NoError(t, m.Install())
			path := tt.unitPath(home)
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(content), "/usr/local/bin/agent")
			assert.Contains(t, string(content), tt.wantContent)
			assert.True(t, rec.sawCommand(tt.installCmd...), "expected install command %v in %v", tt.installCmd, rec.calls)

			require.NoError(t, m.Uninstall())
			_, statErr := os.Stat(path)
			assert.True(t, os.IsNotExist(statErr), "unit file should be removed")
			assert.True(t, rec.sawCommand(tt.uninstall...), "expected uninstall command %v in %v", tt.uninstall, rec.calls)
		})
	}
}

func TestServiceManagerUnsupportedOS(t *testing.T) {
	m := &ServiceManager{goos: "windows", homeDir: t.TempDir(), exePath: "/x/agent", run: func(string, ...string) error { return nil }}
	assert.Error(t, m.Install())
	assert.Error(t, m.Uninstall())
}
