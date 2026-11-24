package engineops_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/hive/engineops"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/tasks"
)

func opNamed(t *testing.T, h *engineops.Handlers, name string) engineops.Op {
	t.Helper()
	for _, op := range h.Ops() {
		if op.Spec.Info().Name == name {
			return op
		}
	}
	t.Fatalf("no engine op %s", name)
	return engineops.Op{}
}

func createEpicOn(t *testing.T, f fleetFixture, server, caller string) hc.Item {
	t.Helper()
	out, err := f.h.CallOn(t.Context(), opNamed(t, f.h, tasks.TaskCreate.Name), server, map[string]any{
		"caller": caller, "repo": "acme/site", "title": "Ship", "type": "epic",
		"children": []map[string]any{{"title": "First"}},
	})
	require.NoError(t, err)
	created, ok := out.(tasks.TaskListOut)
	require.True(t, ok, "a remote answer decodes to the handler's type, got %T", out)
	require.Len(t, created.Tasks, 2)
	return created.Tasks[0]
}

func refusalKind(t *testing.T, err error) ops.Kind {
	t.Helper()
	var opErr *ops.Error
	require.ErrorAs(t, err, &opErr)
	return opErr.Kind
}

func TestCallOnRunsTheOperationOnTheNamedServer(t *testing.T) {
	f := newFleetFixture(t)
	epic := createEpicOn(t, f, "box", t.TempDir())

	out, err := f.h.CallOn(t.Context(), opNamed(t, f.h, tasks.TaskNext.Name), "box", map[string]any{"caller": t.TempDir(), "epicId": epic.ID})
	require.NoError(t, err)
	next, ok := out.(hc.Item)
	require.True(t, ok, "got %T", out)
	assert.Equal(t, "First", next.Title)

	_, err = f.h.CallOn(t.Context(), opNamed(t, f.h, tasks.TaskGet.Name), "", map[string]any{"taskId": epic.ID})
	assert.Equal(t, ops.KindNotFound, refusalKind(t, err), "the epic lives on box, not here")
}

func TestCallOnFollowsAQualifiedCaller(t *testing.T) {
	f := newFleetFixture(t)
	epic := createEpicOn(t, f, "box", "global")

	out, err := f.h.CallOn(t.Context(), opNamed(t, f.h, tasks.TaskNext.Name), "", map[string]any{
		"caller": "box:" + f.boxed.ID, "epicId": epic.ID, "assign": true,
	})

	require.NoError(t, err)
	claimed := out.(hc.Item)
	assert.Equal(t, f.boxed.ID, claimed.SessionID, "the server reads the caller's id as its own session")
}

func TestCallOnRefusesWhatItCannotRoute(t *testing.T) {
	f := newFleetFixture(t)
	next := opNamed(t, f.h, tasks.TaskNext.Name)

	_, err := f.h.CallOn(t.Context(), next, "down", map[string]any{"caller": "box:abc", "epicId": "hc-1"})
	assert.Equal(t, ops.KindInvalid, refusalKind(t, err), "the caller and the server disagree")

	_, err = f.h.CallOn(t.Context(), next, "nowhere", map[string]any{"caller": "global", "epicId": "hc-1"})
	assert.Equal(t, ops.KindNotFound, refusalKind(t, err))

	_, err = f.h.CallOn(t.Context(), next, "down", map[string]any{"caller": "global", "epicId": "hc-1"})
	assert.Equal(t, ops.KindUnavailable, refusalKind(t, err))
	assert.Contains(t, err.Error(), "Connection refused", "ssh's reason reaches the error")

	_, err = f.h.CallOn(t.Context(), opNamed(t, f.h, tasks.TaskGet.Name), "box", map[string]any{"taskId": "hc-nope"})
	assert.Equal(t, ops.KindNotFound, refusalKind(t, err), "the server's refusal keeps its kind")
}

func TestCallOnRunsALocalQualifiedCallerHere(t *testing.T) {
	f := newFleetFixture(t)
	epic := createEpicOn(t, f, "", "global")

	out, err := f.h.CallOn(t.Context(), opNamed(t, f.h, tasks.TaskNext.Name), "", map[string]any{
		"caller": "local:" + f.local.ID, "epicId": epic.ID, "assign": true,
	})

	require.NoError(t, err)
	assert.Equal(t, f.local.ID, out.(hc.Item).SessionID)
}
