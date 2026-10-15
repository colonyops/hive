package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func configYAML(listen, matches string) string {
	return "base_url: https://drive.example.com\nlisten: \"" + listen + "\"\nuser_policies:\n  - name: p\n    matches: [\"" + matches + "\"]\n    quotas:\n      limit: 1GiB\n"
}

// saveByRename writes like an editor that replaces the file atomically.
func saveByRename(t *testing.T, dir, content string) {
	t.Helper()
	tmp := filepath.Join(dir, ".drive-server.yaml.tmp")
	require.NoError(t, os.WriteFile(tmp, []byte(content), 0o600))
	require.NoError(t, os.Rename(tmp, filepath.Join(dir, config.ConfigFileName)))
}

type reloadRig struct {
	dir      string
	policies *core.Policies
	logs     *syncBuffer
}

func startReload(t *testing.T) reloadRig {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, config.ConfigFileName), []byte(configYAML(":8080", "*@old.example")), 0o600))

	load := func() (config.Config, []string, error) {
		cfg, err := config.Load(func(string) string { return "" }, config.Flags{ConfigDir: dir})
		if err != nil {
			return config.Config{}, nil, err
		}
		warnings, err := cfg.Validate()
		return cfg, warnings, err
	}
	running, _, err := load()
	require.NoError(t, err)

	rig := reloadRig{dir: dir, policies: core.NewPolicies(running.UserPolicies), logs: &syncBuffer{}}
	log := zerolog.New(rig.logs)
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(t.Context())
	go func() { done <- watchConfig(ctx, log, running, rig.policies, load) }()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-done)
	})
	// dirwatch starts after watchConfig is scheduled; let it register.
	time.Sleep(100 * time.Millisecond)
	return rig
}

func TestReloadSwapsPoliciesOnRenameSave(t *testing.T) {
	rig := startReload(t)
	_, ok := rig.policies.Match("a@new.example")
	require.False(t, ok)

	saveByRename(t, rig.dir, configYAML(":8080", "*@new.example"))

	require.Eventually(t, func() bool {
		_, ok := rig.policies.Match("a@new.example")
		return ok
	}, 2*time.Second, 20*time.Millisecond)
	_, ok = rig.policies.Match("a@old.example")
	assert.False(t, ok)
}

func TestReloadKeepsPoliciesOnInvalidFile(t *testing.T) {
	rig := startReload(t)

	saveByRename(t, rig.dir, "user_policies: [not a policy")

	require.Eventually(t, func() bool {
		return strings.Contains(rig.logs.String(), `"level":"warn"`)
	}, 2*time.Second, 20*time.Millisecond)
	_, ok := rig.policies.Match("a@old.example")
	assert.True(t, ok)
}

func TestReloadLogsRestartRequiredForListen(t *testing.T) {
	rig := startReload(t)

	saveByRename(t, rig.dir, configYAML(":9090", "*@old.example"))

	require.Eventually(t, func() bool {
		out := rig.logs.String()
		return strings.Contains(out, "restart required") && strings.Contains(out, `"field":"listen"`)
	}, 2*time.Second, 20*time.Millisecond)
}

func TestReloadMissingDirectoryBlocksUntilDone(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	ctx, cancel := context.WithCancel(t.Context())
	logs := &syncBuffer{}
	done := make(chan error, 1)
	go func() {
		done <- watchConfig(ctx, zerolog.New(logs), config.Config{ConfigDir: missing}, core.NewPolicies(nil), nil)
	}()

	require.Eventually(t, func() bool {
		return strings.Contains(logs.String(), "config directory missing; policies will not reload")
	}, time.Second, 10*time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("returned early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	cancel()
	require.NoError(t, <-done)
	_, err := os.Stat(missing)
	assert.True(t, os.IsNotExist(err))
}
