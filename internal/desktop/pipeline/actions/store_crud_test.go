package actions

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
)

type usageStub struct {
	usage ActionUsage
	err   error
}

func (u usageStub) Usage(string) (ActionUsage, error) { return u.usage, u.err }

func editableShell(id string) EditableAction {
	return EditableAction{ID: id, Label: id, Type: "shell", ShowInDetail: true, Shell: &EditableShellConfig{
		CommandTemplate: "echo ok", Cwd: "/tmp", Timeout: "2s", Env: map[string]string{"ZED": "z", "ALPHA": "a"},
	}}
}

func TestActionStoreCRUDPreservesUnrelatedNodesCommentsOrderAndValidatesFullCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.yml")
	original := `# catalog header
version: 1
actions:
  # first action comment
  - id: first
    label: First
    type: shell
    command_template: "true"
  # updated-entry-comment-is-intentionally-replaced
  - id: edit-me
    label: Old
    type: launch-session
    prompt_template: Old
  # trailing action comment
  - id: last
    label: Last
    type: publish-event
    topic: events
`
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
	s := NewActionStore(path)

	created, err := s.Create(editableShell("new-action"))
	require.NoError(t, err)
	assert.Equal(t, "new-action", created.ID)
	got := string(mustRead(t, path))
	assert.Contains(t, got, "# catalog header")
	assert.Contains(t, got, "# first action comment")
	assert.Contains(t, got, "# trailing action comment")
	assert.Less(t, strings.Index(got, "id: last"), strings.Index(got, "id: new-action"))
	assert.Less(t, strings.Index(got, "ALPHA:"), strings.Index(got, "ZED:"), "env output is deterministic")

	updated := launchEditable("edit-me")
	updated.Label = "New"
	_, err = s.Update("edit-me", updated)
	require.NoError(t, err)
	got = string(mustRead(t, path))
	assert.Less(t, strings.Index(got, "id: first"), strings.Index(got, "id: edit-me"))
	assert.Less(t, strings.Index(got, "id: edit-me"), strings.Index(got, "id: last"))
	assert.NotContains(t, got, "updated-entry-comment-is-intentionally-replaced")
	assert.Contains(t, got, "# trailing action comment")

	require.NoError(t, s.Delete("edit-me"))
	got = string(mustRead(t, path))
	assert.NotContains(t, got, "id: edit-me")
	assert.Contains(t, got, "# first action comment")
	assert.Contains(t, got, "# trailing action comment")

	_, err = s.Create(editableShell("new-action"))
	require.ErrorContains(t, err, "already exists")
	_, err = s.Update("edit-me", editableShell("renamed"))
	require.ErrorContains(t, err, "immutable")

	// An invalid unrelated entry makes the complete candidate invalid, and no
	// mutation is installed over bytes the user must repair first.
	invalid := []byte("version: 1\nactions:\n  - id: bad\n    label: Bad\n    type: shell\n    command_template: ok\n    unknown: nope\n")
	require.NoError(t, os.WriteFile(path, invalid, 0o600))
	_, err = s.Create(editableShell("blocked"))
	require.ErrorContains(t, err, "invalid content")
	assert.Equal(t, invalid, mustRead(t, path))
}

func TestActionStoreExternalEditsKeepLastGoodAndRepairRecovers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte("version: 1\nactions:\n  - id: good\n    label: Good\n    type: shell\n    command_template: true\n"), 0o600))
	s := NewActionStore(path)
	require.Len(t, s.List(), 1)

	require.NoError(t, os.WriteFile(path, []byte("version: 1\nactions: ["), 0o600))
	require.Error(t, s.Reload())
	catalog := s.ListEditable()
	assert.Equal(t, []string{"good"}, []string{catalog.Actions[0].ID})
	assert.NotEmpty(t, catalog.Error)

	require.NoError(t, os.WriteFile(path, []byte("version: 1\nactions:\n  - id: repaired\n    label: Repaired\n    type: publish-event\n    topic: repaired\n"), 0o600))
	require.NoError(t, s.Reload())
	catalog = s.ListEditable()
	assert.Equal(t, "repaired", catalog.Actions[0].ID)
	assert.Empty(t, catalog.Error)
}

func TestActionStoreMutatesMissingAndEmptyFilesAndDeleteLeavesEmptyCatalog(t *testing.T) {
	for _, initial := range [][]byte{nil, []byte(" \n")} {
		t.Run("initial", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "actions.yml")
			if initial != nil {
				require.NoError(t, os.WriteFile(path, initial, 0o600))
			}
			s := NewActionStore(path)
			_, err := s.Create(editableShell("only"))
			require.NoError(t, err)
			require.NoError(t, s.Delete("only"))
			assert.Empty(t, s.List())
			got := string(mustRead(t, path))
			assert.Contains(t, got, "actions: []")
			assert.NotContains(t, got, "review-pr", "deleting the last action never reseeds")
		})
	}
}

func TestActionStoreDeleteReportsUsageBlockers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte("version: 1\nactions:\n  - id: used\n    label: Used\n    type: shell\n    command_template: true\n"), 0o600))
	s := NewActionStore(path)
	s.SetUsageChecker(usageStub{usage: ActionUsage{FlowIDs: []string{"flow-a", "flow-b"}, ActiveCommands: 2}})
	err := s.Delete("used")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flow-a, flow-b")
	assert.Contains(t, err.Error(), "2 nonterminal output command")
	assert.Len(t, s.List(), 1)
}

type blockingActionRefs struct {
	store   *ActionStore
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingActionRefs) ResolveAction(id string) bool {
	first := false
	r.once.Do(func() { first = true })
	if first {
		close(r.entered)
		<-r.release
	}
	_, ok := r.store.Get(id)
	return ok
}

type flowUsageChecker struct {
	flows   *flow.FlowStore
	entered chan struct{}
}

func (c flowUsageChecker) Usage(string) (ActionUsage, error) {
	close(c.entered)
	return ActionUsage{FlowIDs: func() []string {
		out := make([]string, 0)
		for _, f := range c.flows.List() {
			out = append(out, f.ID)
		}
		return out
	}()}, nil
}

func TestActionStoreDeleteDoesNotInvertActionAndFlowLocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte("version: 1\nactions:\n  - id: used\n    label: Used\n    type: shell\n    command_template: true\n"), 0o600))
	actions := NewActionStore(path)
	refs := &blockingActionRefs{store: actions, entered: make(chan struct{}), release: make(chan struct{})}
	flows := flow.NewFlowStore(t.TempDir(), refs)
	f := flow.Flow{ID: "flow-a", Name: "Flow A", Enabled: true, Nodes: []flow.Node{
		{ID: "source", Type: "github-source", Config: &flow.GithubSourceConfig{Kind: "search", Query: "is:open"}},
		{ID: "action", Type: "action", Config: &flow.ActionConfig{Action: "used"}},
	}, Wires: []flow.Wire{{From: "source", To: "action"}}}

	saved := make(chan error, 1)
	go func() { saved <- flows.Save(f) }()
	select {
	case <-refs.entered:
	case <-time.After(time.Second):
		t.Fatal("flow save did not begin action validation")
	}

	usageEntered := make(chan struct{})
	actions.SetUsageChecker(flowUsageChecker{flows: flows, entered: usageEntered})
	deleted := make(chan error, 1)
	go func() { deleted <- actions.Delete("used") }()
	select {
	case <-usageEntered:
	case <-time.After(time.Second):
		t.Fatal("delete did not start its usage preflight")
	}
	close(refs.release)

	select {
	case err := <-saved:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("flow save deadlocked while resolving the action")
	}
	select {
	case err := <-deleted:
		require.ErrorContains(t, err, "flow-a")
	case <-time.After(time.Second):
		t.Fatal("action delete deadlocked while checking flow usage")
	}
}

func withActionFileOps(t *testing.T, update func(*actionFileOps)) {
	t.Helper()
	original := actionFS
	patched := original
	update(&patched)
	actionFS = patched
	t.Cleanup(func() { actionFS = original })
}

func TestAtomicWriteFailureCleansTemporaryFileAndDoesNotInstallTarget(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch func(*actionFileOps)
	}{
		{"directory-create", func(ops *actionFileOps) {
			ops.mkdirAll = func(string, os.FileMode) error { return errors.New("directory create failed") }
		}},
		{"temp-create", func(ops *actionFileOps) {
			ops.createTemp = func(string, string) (*os.File, error) { return nil, errors.New("temp create failed") }
		}},
		{"write", func(ops *actionFileOps) {
			ops.write = func(*os.File, []byte) (int, error) { return 0, errors.New("write failed") }
		}},
		{"sync", func(ops *actionFileOps) { ops.sync = func(*os.File) error { return errors.New("sync failed") } }},
		{"install", func(ops *actionFileOps) {
			ops.rename = func(string, string) error { return errors.New("install failed") }
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "actions.yml")
			withActionFileOps(t, tc.patch)
			err := atomicWrite(path, []byte("version: 1\nactions: []\n"))
			require.Error(t, err)
			assert.NoFileExists(t, path)
			temps, globErr := filepath.Glob(filepath.Join(dir, ".actions-*"))
			require.NoError(t, globErr)
			assert.Empty(t, temps)
		})
	}
}

func TestActionStoreReloadsInstalledFileAfterDirectorySyncFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.yml")
	require.NoError(t, os.WriteFile(path, []byte("version: 1\nactions:\n  - id: run\n    label: Old\n    type: shell\n    auto_apply: true\n    command_template: true\n"), 0o600))
	s := NewActionStore(path)
	require.Equal(t, "Old", s.ListEditable().Actions[0].Label)

	dir := filepath.Dir(path)
	withActionFileOps(t, func(ops *actionFileOps) {
		sync := ops.sync
		ops.sync = func(f *os.File) error {
			if f.Name() == dir {
				return errors.New("directory sync failed")
			}
			return sync(f)
		}
	})
	updated := editableShell("run")
	updated.Label = "New"
	_, err := s.Update("run", updated)
	require.ErrorContains(t, err, "sync actions directory")
	assert.Equal(t, "New", s.ListEditable().Actions[0].Label, "state follows the installed file after rename")
	loaded, err := LoadActions(path)
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.True(t, loaded[0].AutoApply, "hidden legacy setting survives editor updates")
}
