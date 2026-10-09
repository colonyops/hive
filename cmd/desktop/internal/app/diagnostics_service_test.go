package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/diagnostics"
	"github.com/colonyops/hive/cmd/desktop/internal/app/hiveconf"
	"github.com/colonyops/hive/cmd/desktop/internal/app/report"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func diagnosticsFixture(t *testing.T) *DiagnosticsService {
	t.Helper()
	dir := t.TempDir()
	db, err := queries.Open(t.Context(), dir, queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	st := stores.New(db, stores.Options{Now: func() time.Time { return time.Date(2026, 10, 7, 0, 17, 18, 0, time.UTC) }})
	job := newJobService(st.Jobs, newTestBus(t), zerolog.Nop())
	id := job.Begin(t.Context(), "Create session", "new-session", "review 347")
	job.Fail(t.Context(), id, `a session named "review 347" already exists`)
	path := filepath.Join(dir, "desktop.log")
	require.NoError(t, os.WriteFile(path, []byte("2026-10-07T00:17:17Z INF creating session session_name=review-347\n2026-10-07T00:17:19Z INF request complete status=201\n"), 0o600))
	return &DiagnosticsService{paths: settings.Paths{LogFile: path, HiveDataDir: dir, ReportsDir: filepath.Join(dir, "reports")}, jobs: job, build: report.Build{Version: "test"}, environ: func(context.Context) []string { return nil }, webhooks: &WebhookService{}}
}

func TestDiagnosticsCombinesJobFailuresWithoutAgent(t *testing.T) {
	svc := diagnosticsFixture(t)
	out, err := svc.Read(t.Context(), DiagnosticsQuery{OmitRoutine: true})
	require.NoError(t, err)
	require.Len(t, out.Entries, 2)
	require.Equal(t, "desktop", out.Entries[0].Source)
	require.Equal(t, "job-1", out.Entries[1].ID)
	require.Equal(t, "failed", out.Entries[1].Fields["status"])
	require.Equal(t, "new-session", out.Entries[1].Fields["action_id"])
	require.Contains(t, out.Entries[1].Message, "already exists")
	require.NotEmpty(t, out.Sources[1].Error, "missing CLI log must not hide other evidence")
	filtered, err := svc.Read(t.Context(), DiagnosticsQuery{Level: "error", Search: "347"})
	require.NoError(t, err)
	require.Len(t, filtered.Entries, 1)
	filtered, err = svc.Read(t.Context(), DiagnosticsQuery{Search: "action_id"})
	require.NoError(t, err)
	require.Len(t, filtered.Entries, 1)
	require.Equal(t, "job-1", filtered.Entries[0].ID)
	context, err := svc.Context(t.Context(), DiagnosticsIncident{Description: "Nothing appeared", Query: DiagnosticsQuery{Search: "347"}})
	require.NoError(t, err)
	require.Contains(t, context.Text, "already exists")
	require.Contains(t, context.Text, "Nothing appeared")
	saved, err := svc.Save(t.Context(), DiagnosticsIncident{})
	require.NoError(t, err)
	info, err := os.Stat(saved.Path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestDiagnosticsBoundsAndReferences(t *testing.T) {
	svc := diagnosticsFixture(t)
	out, err := svc.Read(t.Context(), DiagnosticsQuery{Limit: 1})
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)
	require.True(t, out.Truncated)
	out, err = svc.Read(t.Context(), DiagnosticsQuery{Reference: "job-1", Search: "does not match", Limit: 1})
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)
	require.Equal(t, "job-1", out.Entries[0].ID)
	require.True(t, out.Truncated)
	_, err = svc.Read(t.Context(), DiagnosticsQuery{Reference: "job-999"})
	require.Equal(t, KindNotFound, KindOf(err))
	_, err = svc.Read(t.Context(), DiagnosticsQuery{Since: "invalid"})
	require.Equal(t, KindInvalid, KindOf(err))
	out, err = svc.Read(t.Context(), DiagnosticsQuery{Since: "2026-10-07T00:17:18Z", Until: "2026-10-07T00:17:18Z"})
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)

	require.NoError(t, os.WriteFile(svc.paths.LogFile, []byte("untimestamped evidence\n"), 0o600))
	out, err = svc.Read(t.Context(), DiagnosticsQuery{Until: "2026-10-07T00:17:18Z"})
	require.NoError(t, err)
	for _, entry := range out.Entries {
		require.NotEmpty(t, entry.Time)
	}
}

func TestDiagnosticsReferenceResponseStaysWithinByteBudget(t *testing.T) {
	fields := make(map[string]string, 64)
	for i := range 64 {
		fields[fmt.Sprintf("field_%d", i)] = strings.Repeat("x", 4<<10)
	}
	entries := make([]diagnostics.Entry, 5)
	for i := range entries {
		entries[i] = diagnostics.NewEntry("desktop", string(rune('a'+i)), "", "info", strings.Repeat("m", 16<<10), strings.Repeat("r", 16<<10), fields)
	}

	bounded, truncated := boundDiagnosticsEntries(entries, len(entries), 2)
	require.True(t, truncated)
	require.Contains(t, bounded, entries[2])
	total := 0
	for _, entry := range bounded {
		data, err := json.Marshal(entry)
		require.NoError(t, err)
		total += len(data)
	}
	require.LessOrEqual(t, total, 512<<10)
}

func TestDiagnosticsPreparationUsesDefaultAndOverrideWithoutSpawning(t *testing.T) {
	core, _, workspace := hiveSetupApp(t)
	_, err := core.HiveConfig.Save(t.Context(), HiveSetupRequest{
		DefaultAgent: "codex", Profiles: []hiveconf.Profile{{Name: "codex", Command: "custom-codex", Flags: []string{"--model", "custom model"}}, {Name: "claude", Command: "custom-claude"}}, Workspaces: []string{workspace},
	})
	require.NoError(t, err)
	result, err := core.Diagnostics.Prepare(t.Context(), DiagnosticsIncident{Description: "window vanished"})
	require.NoError(t, err)
	require.Contains(t, result.Command, "custom-codex")
	data, err := os.ReadFile(result.Path)
	require.NoError(t, err)
	require.Contains(t, string(data), "window vanished")
	t.Setenv("HIVE_DEFAULT_AGENT", "claude")
	result, err = core.Diagnostics.Prepare(t.Context(), DiagnosticsIncident{})
	require.NoError(t, err)
	require.Contains(t, result.Command, "custom-claude")
	result, err = core.Diagnostics.Prepare(t.Context(), DiagnosticsIncident{Agent: "codex"})
	require.NoError(t, err)
	require.Contains(t, result.Command, "custom-codex")
	_, err = core.Diagnostics.Prepare(t.Context(), DiagnosticsIncident{Agent: "missing"})
	require.Equal(t, KindUnavailable, KindOf(err))
	require.Empty(t, core.popupTerminals.List())
}

func TestDiagnosticsSharedLogIsReadOnceAndFilteredByService(t *testing.T) {
	svc := diagnosticsFixture(t)
	svc.paths.LogFile = filepath.Join(svc.paths.HiveDataDir, "hive.log")
	content := "2026-10-07T00:17:17Z INF desktop event service_name=hive-desktop\n" +
		"2026-10-07T00:17:18Z ERR cli event service_name=hive-cli\n" +
		`{"time":"2026-10-07T00:17:19Z","level":"info","message":"json cli event","service_name":"hive-cli"}` + "\n"
	require.NoError(t, os.WriteFile(svc.paths.LogFile, []byte(content), 0o600))
	all, err := svc.Read(t.Context(), DiagnosticsQuery{})
	require.NoError(t, err)
	require.Len(t, all.Entries, 4)
	require.Equal(t, all.Sources[0].Path, all.Sources[1].Path)
	require.Empty(t, all.Sources[1].Error)
	cli, err := svc.Read(t.Context(), DiagnosticsQuery{Source: "cli"})
	require.NoError(t, err)
	require.Len(t, cli.Entries, 2)
	for _, entry := range cli.Entries {
		require.Equal(t, "cli", entry.Source)
		require.Contains(t, entry.ID, "cli-")
	}
	desktop, err := svc.Read(t.Context(), DiagnosticsQuery{Source: "desktop"})
	require.NoError(t, err)
	require.Len(t, desktop.Entries, 1)
	require.Equal(t, "desktop", desktop.Entries[0].Source)
}
