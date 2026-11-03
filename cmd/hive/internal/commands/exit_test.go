package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/cmd/hive/internal/config"
	"github.com/colonyops/hive/internal/hive/hivetest"
)

func testApp(t *testing.T) *app.App {
	t.Helper()
	cfg := &config.Config{Config: *hivetest.LoadConfig(t, t.TempDir())}
	h := hivetest.NewEngine(t, hivetest.Options{Config: &cfg.Config})
	return app.NewApp(zerolog.Nop(), h.Engine, cfg, nil, nil, nil, nil, "")
}

func runRoot(t *testing.T, register func(*cli.Command) *cli.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := register(&cli.Command{Name: "hive", Writer: &out, ErrWriter: &bytes.Buffer{}})
	err := root.Run(context.Background(), append([]string{"hive"}, args...))
	return out.String(), err
}

func TestBatchInvalidInputExitsNonZero(t *testing.T) {
	file := filepath.Join(t.TempDir(), "batch.json")
	require.NoError(t, os.WriteFile(file, []byte("{not json"), 0o600))

	_, err := runRoot(t, NewBatchCmd(&Flags{}, testApp(t)).Register, "batch", "-f", file)

	require.ErrorIs(t, err, errAlreadyReported)
}

func TestSessionInfoOutsideASessionFails(t *testing.T) {
	t.Chdir(t.TempDir())

	t.Run("text", func(t *testing.T) {
		_, err := runRoot(t, NewSessionCmd(&Flags{}, testApp(t)).Register, "session", "info")
		require.ErrorContains(t, err, "not in a hive session")
	})

	t.Run("json", func(t *testing.T) {
		out, err := runRoot(t, NewSessionCmd(&Flags{}, testApp(t)).Register, "session", "info", "--json")
		require.ErrorIs(t, err, errAlreadyReported)
		assert.JSONEq(t, `{"error":"not in a hive session"}`, out)
	})
}

func TestDocMessagingNamesRealFlags(t *testing.T) {
	var buf bytes.Buffer
	printMessagingGuide(&buf)
	guide := buf.String()

	assert.NotContains(t, guide, "--new")
	assert.False(t, strings.Contains(guide, "hive ls"), "the guide points at hive session list")
}
