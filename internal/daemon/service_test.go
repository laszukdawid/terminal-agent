package daemon

import (
	"bytes"
	"encoding/xml"
	"io"
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
			wantContent: `ExecStart="/usr/local/bin/agent" daemon start`,
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

func TestServiceTemplatesEscapeSpecialCharsLinux(t *testing.T) {
	t.Setenv(routines.DataDirEnv, t.TempDir())
	home := t.TempDir()
	m := &ServiceManager{goos: "linux", homeDir: home, exePath: "/opt/My Apps/agent%x", uid: 501, run: func(string, ...string) error { return nil }}
	require.NoError(t, m.Install())

	content, err := os.ReadFile(home + "/.config/systemd/user/" + systemdUnit)
	require.NoError(t, err)
	// Spaces are quoted and % is escaped to %% for systemd.
	assert.Contains(t, string(content), `ExecStart="/opt/My Apps/agent%%x" daemon start`)
}

func TestServiceTemplatesProduceValidPlistXML(t *testing.T) {
	t.Setenv(routines.DataDirEnv, t.TempDir())
	home := t.TempDir()
	m := &ServiceManager{goos: "darwin", homeDir: home, exePath: "/opt/a&b/agent", uid: 501, run: func(string, ...string) error { return nil }}
	require.NoError(t, m.Install())

	content, err := os.ReadFile(home + "/Library/LaunchAgents/" + launchdLabel + ".plist")
	require.NoError(t, err)
	// The plist must remain well-formed XML even with an & in the path.
	decoder := xml.NewDecoder(bytes.NewReader(content))
	for {
		_, tokErr := decoder.Token()
		if tokErr == io.EOF {
			break
		}
		require.NoError(t, tokErr, "plist should be valid XML")
	}
	assert.Contains(t, string(content), "a&amp;b")
}

func TestServiceManagerUnsupportedOS(t *testing.T) {
	m := &ServiceManager{goos: "windows", homeDir: t.TempDir(), exePath: "/x/agent", run: func(string, ...string) error { return nil }}
	assert.Error(t, m.Install())
	assert.Error(t, m.Uninstall())
}
