package daemon

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	LaunchdLabel    = "com.colonyops.hive.daemon"
	SystemdUnitName = "hive-daemon.service"
)

// Unit is an OS service definition for the daemon and the file it belongs in.
type Unit struct {
	Path     string
	Contents string
}

// LaunchdUnit is a launchd agent that keeps `<binary> daemon run --wait-lock`
// alive. --wait-lock queues it behind a daemon a client spawned, instead of
// exiting and being restarted in a loop. AbandonProcessGroup keeps launchd
// from killing a tmux server the daemon started when the daemon exits.
func LaunchdUnit(home, binary, dataDir, configPath string) Unit {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`)
	key := func(k string) { fmt.Fprintf(&b, "\t<key>%s</key>\n", k) }
	str := func(indent, v string) { fmt.Fprintf(&b, "%s<string>%s</string>\n", indent, xmlEscape(v)) }

	key("Label")
	str("\t", LaunchdLabel)
	key("ProgramArguments")
	b.WriteString("\t<array>\n")
	for _, arg := range []string{binary, "daemon", "run", "--wait-lock"} {
		str("\t\t", arg)
	}
	b.WriteString("\t</array>\n")
	key("EnvironmentVariables")
	b.WriteString("\t<dict>\n")
	b.WriteString("\t\t<key>HIVE_CONFIG</key>\n")
	str("\t\t", configPath)
	b.WriteString("\t\t<key>HIVE_DATA_DIR</key>\n")
	str("\t\t", dataDir)
	b.WriteString("\t</dict>\n")
	key("WorkingDirectory")
	str("\t", dataDir)
	key("RunAtLoad")
	b.WriteString("\t<true/>\n")
	key("KeepAlive")
	b.WriteString("\t<true/>\n")
	key("AbandonProcessGroup")
	b.WriteString("\t<true/>\n")
	key("StandardOutPath")
	str("\t", OutLogPath(dataDir))
	key("StandardErrorPath")
	str("\t", OutLogPath(dataDir))
	b.WriteString("</dict>\n</plist>\n")

	return Unit{
		Path:     filepath.Join(home, "Library", "LaunchAgents", LaunchdLabel+".plist"),
		Contents: b.String(),
	}
}

// SystemdUnit is a systemd user unit for `<binary> daemon run --wait-lock`.
// KillMode=process stops only the daemon on a restart, not a tmux server it
// started in its cgroup.
func SystemdUnit(home, binary, dataDir, configPath string) Unit {
	contents := fmt.Sprintf(`[Unit]
Description=hive daemon

[Service]
ExecStart=%s daemon run --wait-lock
Environment=%s
Environment=%s
WorkingDirectory=%s
Restart=always
RestartSec=2
KillMode=process

[Install]
WantedBy=default.target
`,
		systemdQuote(binary),
		systemdQuote("HIVE_CONFIG="+configPath),
		systemdQuote("HIVE_DATA_DIR="+dataDir),
		strings.ReplaceAll(dataDir, "%", "%%"),
	)
	return Unit{
		Path:     filepath.Join(home, ".config", "systemd", "user", SystemdUnitName),
		Contents: contents,
	}
}

// OutLogPath is where a detached daemon's stdout and stderr go.
func OutLogPath(dataDir string) string {
	return filepath.Join(dataDir, "logs", "hived.out")
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// systemdQuote quotes one word for a unit file, where % starts a specifier.
func systemdQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `%`, `%%`)
	return `"` + r.Replace(s) + `"`
}
