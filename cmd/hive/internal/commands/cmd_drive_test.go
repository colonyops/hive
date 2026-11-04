package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestDriveRemoteCommandsTakeDriveServerFlag(t *testing.T) {
	root := NewDriveCmd(&Flags{}, testApp(t)).Register(&cli.Command{Name: "hive"})
	drive := find(root.Commands, "drive")
	require.NotNil(t, drive)

	for _, name := range []string{"push", "pull", "share"} {
		t.Run(name, func(t *testing.T) {
			sub := find(drive.Commands, name)
			require.NotNil(t, sub)
			assert.True(t, hasFlagNamed(sub.Flags, "drive-server"), "want --drive-server")
			assert.False(t, hasFlagNamed(sub.Flags, "server"), "--server must name a fleet server, not a drive server")
		})
	}
}

func hasFlagNamed(flags []cli.Flag, name string) bool {
	for _, f := range flags {
		for _, n := range f.Names() {
			if n == name {
				return true
			}
		}
	}
	return false
}
