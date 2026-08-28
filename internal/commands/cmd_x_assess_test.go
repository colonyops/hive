package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/colonyops/hive/internal/hive"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestAssessFileCmd_EmitsExpectedJSONShape(t *testing.T) {
	var buf bytes.Buffer

	flags := &Flags{}
	cmd := NewExperimentalCmd(flags, &hive.App{})

	app := &cli.Command{
		Name:   "hive",
		Writer: &buf,
	}
	cmd.Register(app)

	fixture := "../core/terminal/assess/testdata/claude/approval-permission-dialog.txt"
	require.NoError(t, app.Run(context.Background(), []string{"hive", "x", "assess", "file", fixture, "--tool", "claude"}))

	var out map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))

	require.Equal(t, "approval", out["state"])
	require.Equal(t, "claude/permission-dialog", out["ruleID"])
	require.Equal(t, false, out["hold"])

	signals, ok := out["signals"].([]any)
	require.True(t, ok, "signals must be a JSON array")
	require.NotEmpty(t, signals)

	regions, ok := out["regions"].(map[string]any)
	require.True(t, ok, "regions must be a JSON object")
	for _, key := range []string{"aboveBox", "promptBoxBody", "bottomLines", "afterLastRule"} {
		_, present := regions[key]
		require.True(t, present, "regions missing key %q", key)
	}
	require.Contains(t, regions["promptBoxBody"], "Do you want to proceed?")
}

func TestAssessFileCmd_MissingArgErrors(t *testing.T) {
	var buf bytes.Buffer

	flags := &Flags{}
	cmd := NewExperimentalCmd(flags, &hive.App{})

	app := &cli.Command{
		Name:   "hive",
		Writer: &buf,
	}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "x", "assess", "file"})
	require.Error(t, err)
}
