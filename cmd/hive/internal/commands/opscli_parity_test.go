package commands

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/cmd/hive/internal/opscli"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/catalog"
)

// TestEveryDesktopOpHasACommand fails when a desktop operation in the
// catalog has no `hive dt <resource> <verb>`, or when its help drifts from
// the spec.
func TestEveryDesktopOpHasACommand(t *testing.T) {
	root := NewDesktopCmd(&Flags{}, &app.App{Config: &config.Config{}}).Register(&cli.Command{Name: "hive"})
	dt := find(root.Commands, "desktop")
	require.NotNil(t, dt)

	for _, op := range catalog.All() {
		info := op.Info()
		if info.Owner != ops.OwnerDesktop {
			continue
		}
		t.Run(info.Name, func(t *testing.T) {
			path := opscli.Path(info)
			group := find(dt.Commands, path[0])
			require.NotNil(t, group, "no dt %s", path[0])
			cmd := find(group.Commands, path[1])
			require.NotNil(t, cmd, "no dt %s %s", path[0], path[1])

			var help bytes.Buffer
			root := NewDesktopCmd(&Flags{}, &app.App{Config: &config.Config{}}).Register(&cli.Command{Name: "hive", Writer: &help})
			require.NoError(t, root.Run(context.Background(), []string{"hive", "dt", path[0], path[1], "--help"}))
			assert.Contains(t, help.String(), info.Summary)
			if info.Guidance != "" {
				assert.Contains(t, help.String(), info.Guidance[:min(len(info.Guidance), 60)])
			}
		})
	}
}

func find(cmds []*cli.Command, name string) *cli.Command {
	for _, c := range cmds {
		if c.Name == name {
			return c
		}
	}
	return nil
}
