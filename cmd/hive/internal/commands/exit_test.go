package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	"github.com/colonyops/hive/internal/hive/hivetest"
	"github.com/colonyops/hive/internal/ops"
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

	require.Error(t, err)
	assert.Equal(t, ops.KindInvalid, ErrorKind(err))
	assert.Equal(t, 2, ExitCode(err))
}

func TestSessionInfoOutsideASessionFails(t *testing.T) {
	t.Setenv("HIVE_AGENT_SESSION", "")
	t.Chdir(t.TempDir())

	t.Run("text", func(t *testing.T) {
		_, err := runRoot(t, NewSessionCmd(&Flags{}, testApp(t)).Register, "session", "info")
		require.ErrorContains(t, err, "not in a hive session")
	})

	t.Run("json", func(t *testing.T) {
		out, err := runRoot(t, NewSessionCmd(&Flags{}, testApp(t)).Register, "session", "info", "--json")
		require.Error(t, err)
		assert.Empty(t, out, "stdout carries results only")
		assert.Equal(t, ops.KindNotFound, ErrorKind(err))
	})
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"success", nil, 0},
		{"plain error", errors.New("boom"), 1},
		{"exit error", ExitError{Code: 7}, 7},
		{"refused call", &ops.Error{Kind: ops.KindNotFound}, 3},
		{"wrapped refused call", fmt.Errorf("tab open: %w", &ops.Error{Kind: ops.KindConflict}), 4},
		{"usage error", invalidf("name required"), 2},
		{"missing session", fmt.Errorf("get session: %w", session.ErrNotFound), 3},
		{"missing item", fmt.Errorf("show: %w", hc.ErrNotFound), 3},
		{"duplicate session", session.ErrDuplicateName, 4},
		{"unreachable daemon", wire.NewUnreachableError("", "daemon is down", errors.New("dial")), 5},
		{"desktop not running", desktopNotRunningError{}, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ExitCode(tt.err))
		})
	}
}

func TestReportErrorRendersTextOrJSON(t *testing.T) {
	cause := &ops.Error{Kind: ops.KindNotFound, Msg: "no tab"}

	var text bytes.Buffer
	assert.Equal(t, 3, ReportError(&text, &ReportedError{Err: cause, Kind: ops.KindNotFound}))
	assert.Equal(t, "not_found: no tab\n", text.String())

	var line bytes.Buffer
	assert.Equal(t, 3, ReportError(&line, &ReportedError{Err: cause, Kind: ops.KindNotFound, JSON: true}))
	assert.JSONEq(t, `{"kind":"not_found","message":"no tab"}`, line.String())

	var silent bytes.Buffer
	assert.Equal(t, 1, ReportError(&silent, errAlreadyReported))
	assert.Empty(t, silent.String())
}

func TestReportErrorNamesTheServersCommandForAnUnreachableServer(t *testing.T) {
	cause := wire.NewUnreachableError("server-b", "could not reach server-b", errors.New("refused"))

	var text bytes.Buffer
	ReportError(&text, &ReportedError{Err: cause, Kind: ops.KindUnavailable})
	assert.Contains(t, text.String(), "hive servers ls")

	var line bytes.Buffer
	ReportError(&line, &ReportedError{Err: cause, Kind: ops.KindUnavailable, JSON: true})
	assert.Equal(t, 1, strings.Count(line.String(), "\n"), "stderr carries one JSON line")
	var got errorLine
	require.NoError(t, json.Unmarshal(line.Bytes(), &got))
	assert.Equal(t, ops.KindUnavailable, got.Kind)
	assert.Contains(t, got.Message, "hive servers ls")
}

func TestDocMessagingNamesRealFlags(t *testing.T) {
	var buf bytes.Buffer
	printMessagingGuide(&buf)
	guide := buf.String()

	assert.NotContains(t, guide, "--new")
	assert.NotContains(t, guide, "hive ls", "the guide points at hive session list")
}
