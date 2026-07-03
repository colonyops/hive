//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/sources/rpc"
)

var (
	refSourceOnce sync.Once
	refSourcePath string
	refSourceErr  error
)

// buildReferenceSource compiles cmd/hive-reference-source once per
// test process (shared across all tests that need it) and returns the
// binary path. Building rather than using `go run` avoids paying Go's
// build overhead on every one-shot RPC call.
func buildReferenceSource(t *testing.T) string {
	t.Helper()

	refSourceOnce.Do(func() {
		repoRoot, err := filepath.Abs("../..")
		if err != nil {
			refSourceErr = err
			return
		}

		dir, err := os.MkdirTemp("", "hive-reference-source-*")
		if err != nil {
			refSourceErr = err
			return
		}

		binPath := filepath.Join(dir, "hive-reference-source")
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/hive-reference-source")
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			refSourceErr = fmt.Errorf("build reference source: %w: %s", err, out)
			return
		}
		refSourcePath = binPath
	})

	require.NoError(t, refSourceErr)
	return refSourcePath
}

func TestSourceRPCAdapterReferenceSource(t *testing.T) {
	binPath := buildReferenceSource(t)

	conn, err := rpc.NewSubprocessSource("reference", []string{binPath}, rpc.ExecProcessRunner{}, 10*time.Second)
	require.NoError(t, err)

	ctx := context.Background()

	assert.True(t, conn.Available(ctx))

	manifest, err := conn.Initialize(ctx)
	require.NoError(t, err)
	assert.Equal(t, "reference", manifest.ID)
	assert.Equal(t, sources.LayoutModeList, manifest.Picker.Layout)
	assert.Equal(t, sources.SearchModeRemote, manifest.Picker.Search.Mode)
	assert.NotEmpty(t, manifest.Picker.Columns)

	searchResult, err := conn.Search(ctx, sources.SearchParams{Query: "anything"})
	require.NoError(t, err)
	require.Len(t, searchResult.Items, 3)
	assert.Equal(t, "ref-1", searchResult.Items[0].ID)
	assert.Equal(t, "First reference item", searchResult.Items[0].Title)

	detail, err := conn.FetchDetail(ctx, sources.FetchDetailParams{ID: "ref-1"})
	require.NoError(t, err)
	require.NotNil(t, detail.Markdown)
	assert.Contains(t, detail.Markdown.Content, "ref-1")

	unknownDetail, err := conn.FetchDetail(ctx, sources.FetchDetailParams{ID: "does-not-exist"})
	require.NoError(t, err)
	assert.Equal(t, sources.DetailKindNone, unknownDetail.Kind())
}

// writeSpawnCountWrapper writes a shell script that appends one byte to
// countFile every time it is invoked, then execs the reference source
// binary with the same stdio. This lets the test prove the adapter spawns
// the process exactly once per RPC call.
func writeSpawnCountWrapper(t *testing.T, binPath, countFile string) string {
	t.Helper()

	wrapperPath := filepath.Join(t.TempDir(), "spawn-count-wrapper.sh")
	script := fmt.Sprintf("#!/bin/sh\nprintf x >> %q\nexec %q \"$@\"\n", countFile, binPath)
	require.NoError(t, os.WriteFile(wrapperPath, []byte(script), 0o755))
	return wrapperPath
}

func TestSourceRPCAdapterSpawnsOncePerCall(t *testing.T) {
	binPath := buildReferenceSource(t)
	countFile := filepath.Join(t.TempDir(), "spawn-count")
	wrapperPath := writeSpawnCountWrapper(t, binPath, countFile)

	conn, err := rpc.NewSubprocessSource("reference", []string{wrapperPath}, rpc.ExecProcessRunner{}, 10*time.Second)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = conn.Initialize(ctx)
	require.NoError(t, err)
	_, err = conn.Search(ctx, sources.SearchParams{})
	require.NoError(t, err)
	_, err = conn.FetchDetail(ctx, sources.FetchDetailParams{ID: "ref-1"})
	require.NoError(t, err)

	data, err := os.ReadFile(countFile)
	require.NoError(t, err)
	assert.Len(t, data, 3, "expected exactly one process spawn per RPC call")
}
