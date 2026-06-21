package daemon

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"text/template"

	"github.com/laszukdawid/terminal-agent/internal/routines"
)

// Service identifiers for the daemon's own OS supervisor entry (one per machine,
// not per routine).
const (
	launchdLabel = "com.terminal-agent.daemon"
	systemdUnit  = "terminal-agent.service"
)

// commandRunner runs an external command; injectable so tests don't invoke the
// real launchctl/systemctl.
type commandRunner func(name string, args ...string) error

func defaultCommandRunner(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, bytes.TrimSpace(out))
	}
	return nil
}

// ServiceManager installs and removes the daemon as an OS-managed service so it
// starts on login/boot and restarts on failure.
type ServiceManager struct {
	goos    string
	homeDir string
	exePath string
	uid     int
	run     commandRunner
}

// NewServiceManager builds a ServiceManager for the current platform and user.
func NewServiceManager(exePath string) (*ServiceManager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &ServiceManager{
		goos:    runtime.GOOS,
		homeDir: home,
		exePath: exePath,
		uid:     os.Getuid(),
		run:     defaultCommandRunner,
	}, nil
}

// Install writes the platform service definition and loads it.
func (m *ServiceManager) Install() error {
	switch m.goos {
	case "darwin":
		return m.installLaunchd()
	case "linux":
		return m.installSystemd()
	default:
		return fmt.Errorf("service install is not supported on %s; run `agent daemon start` under your own supervisor", m.goos)
	}
}

// Uninstall unloads and removes the platform service definition.
func (m *ServiceManager) Uninstall() error {
	switch m.goos {
	case "darwin":
		return m.uninstallLaunchd()
	case "linux":
		return m.uninstallSystemd()
	default:
		return fmt.Errorf("service uninstall is not supported on %s", m.goos)
	}
}

func (m *ServiceManager) launchdPlistPath() string {
	return filepath.Join(m.homeDir, "Library", "LaunchAgents", launchdLabel+".plist")
}

func (m *ServiceManager) systemdUnitPath() string {
	return filepath.Join(m.homeDir, ".config", "systemd", "user", systemdUnit)
}

func (m *ServiceManager) installLaunchd() error {
	path := m.launchdPlistPath()
	logPath := filepath.Join(routines.DataDir(), "daemon.log")
	content, err := renderTemplate(launchdPlistTemplate, map[string]string{
		"Label":   launchdLabel,
		"ExePath": m.exePath,
		"LogPath": logPath,
	})
	if err != nil {
		return err
	}
	if err := writeFile(path, content); err != nil {
		return err
	}
	target := "gui/" + strconv.Itoa(m.uid)
	// Replace any prior instance idempotently before loading.
	_ = m.run("launchctl", "bootout", target+"/"+launchdLabel)
	return m.run("launchctl", "bootstrap", target, path)
}

func (m *ServiceManager) uninstallLaunchd() error {
	target := "gui/" + strconv.Itoa(m.uid)
	_ = m.run("launchctl", "bootout", target+"/"+launchdLabel)
	return os.Remove(m.launchdPlistPath())
}

func (m *ServiceManager) installSystemd() error {
	path := m.systemdUnitPath()
	content, err := renderTemplate(systemdUnitTemplate, map[string]string{
		"ExePath": m.exePath,
	})
	if err != nil {
		return err
	}
	if err := writeFile(path, content); err != nil {
		return err
	}
	if err := m.run("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	return m.run("systemctl", "--user", "enable", "--now", systemdUnit)
}

func (m *ServiceManager) uninstallSystemd() error {
	_ = m.run("systemctl", "--user", "disable", "--now", systemdUnit)
	if err := os.Remove(m.systemdUnitPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return m.run("systemctl", "--user", "daemon-reload")
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func renderTemplate(tmpl *template.Template, data map[string]string) (string, error) {
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

var launchdPlistTemplate = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{.Label}}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.ExePath}}</string>
        <string>daemon</string>
        <string>start</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{{.LogPath}}</string>
    <key>StandardErrorPath</key>
    <string>{{.LogPath}}</string>
</dict>
</plist>
`))

var systemdUnitTemplate = template.Must(template.New("unit").Parse(`[Unit]
Description=Terminal Agent routine scheduler
After=default.target

[Service]
Type=simple
ExecStart={{.ExePath}} daemon start
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`))
