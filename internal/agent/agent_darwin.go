package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist"), nil
}

func logPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Logs", "Lumpy", "agent.log")
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func domain() string { return "gui/" + strconv.Itoa(os.Getuid()) }

// Install writes the launchd agent for the lumpy binary at bin and loads
// it. The agent runs at login and every time a volume is mounted.
func Install(bin string, env map[string]string) (string, error) {
	p, err := plistPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(logPath()), 0o755); err != nil {
		return "", err
	}
	var envXML strings.Builder
	if len(env) > 0 {
		envXML.WriteString("\t<key>EnvironmentVariables</key>\n\t<dict>\n")
		for k, v := range env {
			fmt.Fprintf(&envXML, "\t\t<key>%s</key>\n\t\t<string>%s</string>\n", xmlEscape(k), xmlEscape(v))
		}
		envXML.WriteString("\t</dict>\n")
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>agent</string>
		<string>run</string>
	</array>
	<key>StartOnMount</key>
	<true/>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Standard</string>
%s	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, Label, xmlEscape(bin), envXML.String(), xmlEscape(logPath()), xmlEscape(logPath()))
	// Reload cleanly if an older copy is installed.
	exec.Command("launchctl", "bootout", domain()+"/"+Label).Run()
	if err := os.WriteFile(p, []byte(plist), 0o644); err != nil {
		return "", err
	}
	if out, err := exec.Command("launchctl", "bootstrap", domain(), p).CombinedOutput(); err != nil {
		return "", fmt.Errorf("launchctl bootstrap: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return p, nil
}

// Uninstall unloads and removes the agent.
func Uninstall() error {
	p, err := plistPath()
	if err != nil {
		return err
	}
	exec.Command("launchctl", "bootout", domain()+"/"+Label).Run()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Status describes whether the agent is installed and loaded.
func Status() string {
	p, _ := plistPath()
	if _, err := os.Stat(p); err != nil {
		return "not installed (run `lumpy agent install`)"
	}
	if err := exec.Command("launchctl", "print", domain()+"/"+Label).Run(); err != nil {
		return "installed at " + p + " but not loaded (run `lumpy agent install` again)"
	}
	return "installed and running on every drive mount; log: " + logPath()
}

// Notify posts a macOS notification.
func Notify(title, msg string) {
	q := func(s string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"` }
	exec.Command("osascript", "-e", "display notification "+q(msg)+" with title "+q(title)).Run()
}
