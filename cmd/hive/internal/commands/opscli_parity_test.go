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
			require.NotNil(t, lookup(dt, path), "no dt %v", path)

			var help bytes.Buffer
			root := NewDesktopCmd(&Flags{}, &app.App{Config: &config.Config{}}).Register(&cli.Command{Name: "hive", Writer: &help})
			require.NoError(t, root.Run(context.Background(), append(append([]string{"hive", "dt"}, path...), "--help")))
			assert.Contains(t, help.String(), info.Summary)
			if info.Guidance != "" {
				assert.Contains(t, help.String(), info.Guidance[:min(len(info.Guidance), 60)])
			}
		})
	}
}

// TestEveryEngineOpHasACommand fails when an engine operation in the catalog
// has no command below hc, msg or todo, or when its help drifts from the spec.
func TestEveryEngineOpHasACommand(t *testing.T) {
	a := outputTestApp(t)
	for _, op := range catalog.All() {
		info := op.Info()
		if info.Owner != ops.OwnerEngine {
			continue
		}
		t.Run(info.Name, func(t *testing.T) {
			var help bytes.Buffer
			root := registerCommands(&cli.Command{Name: "hive", Writer: &help}, a)
			path := enginePath(info)
			require.NotNil(t, lookup(root, path), "no hive %v", path)

			require.NoError(t, root.Run(context.Background(), append(append([]string{"hive"}, path...), "--help")))
			assert.Contains(t, help.String(), info.Summary)
			assert.Contains(t, help.String(), info.Guidance[:min(len(info.Guidance), 60)])
			assert.Contains(t, help.String(), "--caller", "the group carries --caller")
		})
	}
}

func lookup(c *cli.Command, path []string) *cli.Command {
	for _, name := range path {
		if c = find(c.Commands, name); c == nil {
			return nil
		}
	}
	return c
}

func TestEngineCommandPaths(t *testing.T) {
	root := commandTree(t)
	for _, path := range [][]string{
		{"hc", "create"},
		{"hc", "list"},
		{"hc", "get"},
		{"hc", "update"},
		{"hc", "next"},
		{"hc", "clear"},
		{"hc", "comment", "create"},
		{"hc", "context", "get"},
		{"msg", "create"},
		{"msg", "list"},
		{"msg", "read", "put"},
		{"msg", "topic", "list"},
		{"msg", "topic", "new"},
		{"msg", "sub"},
		{"msg", "inbox"},
		{"todo", "create"},
		{"todo", "list"},
		{"todo", "update"},
	} {
		assert.NotNil(t, lookup(root, path), "no hive %v", path)
	}
	for _, name := range []string{"task", "task-comment", "task-context", "message", "message-read", "topic"} {
		assert.Nil(t, find(root.Commands, name), "hive %s is a root command", name)
	}
}
