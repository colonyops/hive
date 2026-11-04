package daemon_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/hive/daemon"
)

func TestServiceUnits(t *testing.T) {
	t.Parallel()
	const (
		home   = "/home/ada"
		binary = "/opt/hive/bin/hive"
		data   = "/home/ada/.local/share/hive"
		config = "/home/ada/.config/hive/config & 100%.yaml"
	)
	tests := []struct {
		name   string
		unit   daemon.Unit
		path   string
		golden string
	}{
		{"launchd", daemon.LaunchdUnit(home, binary, data, config), "/home/ada/Library/LaunchAgents/com.colonyops.hive.daemon.plist", "launchd.plist.golden"},
		{"systemd", daemon.SystemdUnit(home, binary, data, config), "/home/ada/.config/systemd/user/hive-daemon.service", "systemd.service.golden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.path, tt.unit.Path)
			assert.Contains(t, tt.unit.Contents, "--wait-lock")
			golden := filepath.Join("testdata", tt.golden)
			if os.Getenv("UPDATE_GOLDEN") != "" {
				require.NoError(t, os.WriteFile(golden, []byte(tt.unit.Contents), 0o644))
			}
			want, err := os.ReadFile(golden)
			require.NoError(t, err, "golden file missing; run with UPDATE_GOLDEN=1 to create it")
			assert.Equal(t, string(want), tt.unit.Contents)
		})
	}
}

func TestServiceUnitsSetArgvZeroToHive(t *testing.T) {
	t.Parallel()
	launchd := daemon.LaunchdUnit("/h", "/bin/hive-desktop", "/d", "/c.yaml").Contents
	assert.Contains(t, launchd, "<key>Program</key>\n\t<string>/bin/hive-desktop</string>")
	assert.Contains(t, launchd, "<array>\n\t\t<string>hive</string>\n\t\t<string>daemon</string>")

	systemd := daemon.SystemdUnit("/h", "/bin/hive-desktop", "/d", "/c.yaml").Contents
	assert.Contains(t, systemd, `ExecStart=@"/bin/hive-desktop" hive daemon run --wait-lock`)
}

func TestSystemdUnitKillsOnlyTheDaemon(t *testing.T) {
	t.Parallel()
	unit := daemon.SystemdUnit("/h", "/bin/hive", "/d", "/c.yaml")
	assert.Contains(t, unit.Contents, "KillMode=process")
	assert.Contains(t, unit.Contents, "Restart=always")
}
