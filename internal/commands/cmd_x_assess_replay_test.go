package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/hive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestAssessWatchCmd_MissingArgErrors(t *testing.T) {
	flags := &Flags{}
	cmd := NewExperimentalCmd(flags, &hive.App{})

	app := &cli.Command{Name: "hive"}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "x", "assess", "watch"})
	require.Error(t, err)
}

func TestAssessReplayCmd_MissingArgErrors(t *testing.T) {
	flags := &Flags{}
	cmd := NewExperimentalCmd(flags, &hive.App{})

	app := &cli.Command{Name: "hive"}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "x", "assess", "replay"})
	require.Error(t, err)
}

// TestAssessReplayCmd_Deterministic proves the core claim of the replay
// tool: the same recorded frames, on a fresh engine+tracker driven purely by
// the frames' own timestamps (never wall time), produce byte-identical
// output every run.
func TestAssessReplayCmd_Deterministic(t *testing.T) {
	framesPath := writeAssessTestFrames(t, []assessFrame{
		{Timestamp: assessTestBase, Content: "⠋ Thinking… (working)\n"},
		{Timestamp: assessTestBase.Add(1500 * time.Millisecond), Content: "line one\n❯"},
		{Timestamp: assessTestBase.Add(3000 * time.Millisecond), Content: "line one\n❯"},
		{Timestamp: assessTestBase.Add(4500 * time.Millisecond), Content: "line one\n❯"},
		{Timestamp: assessTestBase.Add(6000 * time.Millisecond), Content: "Some setup text\nContinue? (y/n)"},
	})

	runReplay := func() string {
		var out bytes.Buffer
		flags := &Flags{}
		cmd := NewExperimentalCmd(flags, &hive.App{})
		app := &cli.Command{Name: "hive", Writer: &out}
		cmd.Register(app)

		err := app.Run(context.Background(), []string{
			"hive", "x", "assess", "replay", framesPath, "--tool", "test-tool", "--jsonl",
		})
		require.NoError(t, err)
		return out.String()
	}

	first := runReplay()
	second := runReplay()

	require.NotEmpty(t, first)
	assert.Equal(t, first, second, "replaying the same frames twice must produce identical decision sequences")
}

var assessTestBase = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func writeAssessTestFrames(t *testing.T, frames []assessFrame) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "frames.jsonl")
	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	for _, frame := range frames {
		require.NoError(t, enc.Encode(frame))
	}

	return path
}
