package engineops_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/daemon/daemontest"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	"github.com/colonyops/hive/internal/hive/engineops"
	"github.com/colonyops/hive/internal/hive/hivetest"
	"github.com/colonyops/hive/internal/hive/servers"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/sessions"
	"github.com/colonyops/hive/internal/store"
)

type fleetFixture struct {
	h     *engineops.Handlers
	local session.Session
	boxed session.Session
}

// newFleetFixture is a local engine with one session and a fleet whose server
// box is a test daemon with one session, and whose server down never answers.
func newFleetFixture(t *testing.T) fleetFixture {
	t.Helper()
	harness := hivetest.NewEngine(t, hivetest.Options{})
	local := session.Session{
		ID: "loc001", Name: "local work", Slug: "local-work", Path: t.TempDir(), State: session.StateActive,
		Remote: "git@github.com:acme/widgets.git", Tags: []string{"auto"}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, store.NewSessionStore(harness.DB).Save(t.Context(), local))

	socket := daemontest.Socket(t, hivetest.Mux{})
	box := daemonclient.New(daemonclient.Options{Dialer: daemonclient.UnixDialer(socket), Logger: zerolog.Nop()})
	boxed, err := box.CreateSession(t.Context(), sessionsvc.CreateOptions{
		Name: "remote work", Remote: "https://github.com/acme/site", Tags: []string{"auto"}, SkipSpawn: true,
	})
	require.NoError(t, err)

	dial := func(ctx context.Context, cfg config.ServerConfig, stderr io.Writer) (net.Conn, error) {
		if cfg.Name == "down" {
			_, _ = io.WriteString(stderr, "ssh: connect to host down port 22: Connection refused\n")
			return nil, errors.New("exit status 255")
		}
		var d net.Dialer
		return d.DialContext(ctx, "unix", socket)
	}
	fleet := servers.New(zerolog.Nop(), sessionapi.NewLocal(harness.Engine), []config.ServerConfig{{Name: "box", SSH: "box"}, {Name: "down", SSH: "down"}}, "test", servers.WithDialer(dial))
	t.Cleanup(fleet.Close)

	return fleetFixture{
		h:     engineops.New(zerolog.Nop(), harness.Engine, engineops.WithFleet(func() *servers.Fleet { return fleet })),
		local: local,
		boxed: *boxed,
	}
}

func refs(out sessions.SessionListOut) []string {
	var got []string
	for _, s := range out.Sessions {
		got = append(got, s.Server+":"+s.ID)
	}
	return got
}

func TestSessionListReadsEveryServer(t *testing.T) {
	f := newFleetFixture(t)

	out, err := f.h.SessionList(t.Context(), sessions.SessionListIn{})
	require.NoError(t, err)
	assert.Equal(t, []string{"local:" + f.local.ID, "box:" + f.boxed.ID}, refs(out))
	require.Len(t, out.Offline, 1)
	assert.Equal(t, "down", out.Offline[0].Server)

	local, remote := out.Sessions[0], out.Sessions[1]
	assert.Equal(t, "widgets", local.Repo)
	assert.Equal(t, "agent."+f.local.ID+".inbox", local.Inbox)
	require.NotNil(t, local.Unread)
	assert.Empty(t, remote.Inbox, "a server's inbox never gets this machine's messages")
	assert.Nil(t, remote.Unread)
}

func TestSessionListNamedServer(t *testing.T) {
	f := newFleetFixture(t)

	out, err := f.h.SessionList(t.Context(), sessions.SessionListIn{Server: "box"})
	require.NoError(t, err)
	assert.Equal(t, []string{"box:" + f.boxed.ID}, refs(out))

	_, err = f.h.SessionList(t.Context(), sessions.SessionListIn{Server: "down"})
	assert.Equal(t, ops.KindUnavailable, kindOf(t, err), "the one server named failing fails the call")

	_, err = f.h.SessionList(t.Context(), sessions.SessionListIn{Server: "nowhere"})
	assert.Equal(t, ops.KindNotFound, kindOf(t, err))
}

func TestSessionListFiltersByTagsAndState(t *testing.T) {
	f := newFleetFixture(t)

	out, err := f.h.SessionList(t.Context(), sessions.SessionListIn{Tags: []string{"AUTO"}, Server: "local"})
	require.NoError(t, err)
	assert.Equal(t, []string{"local:" + f.local.ID}, refs(out))

	out, err = f.h.SessionList(t.Context(), sessions.SessionListIn{Tags: []string{"other"}, Server: "local"})
	require.NoError(t, err)
	assert.Empty(t, out.Sessions)
	assert.NotNil(t, out.Sessions, "an empty list is [], not null")

	out, err = f.h.SessionList(t.Context(), sessions.SessionListIn{State: "recycled", Server: "local"})
	require.NoError(t, err)
	assert.Empty(t, out.Sessions)
}

func TestSessionListReportsCorruptedSessionsWhateverTheTags(t *testing.T) {
	harness := hivetest.NewEngine(t, hivetest.Options{})
	broken := session.Session{
		ID: "bad001", Name: "broken", Slug: "broken", Path: t.TempDir(), State: session.StateCorrupted,
		Remote: "git@github.com:acme/widgets.git", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, store.NewSessionStore(harness.DB).Save(t.Context(), broken))
	h := engineops.New(zerolog.Nop(), harness.Engine)

	out, err := h.SessionList(t.Context(), sessions.SessionListIn{Tags: []string{"auto"}})
	require.NoError(t, err)
	assert.Empty(t, out.Sessions)
	require.Len(t, out.Corrupted, 1)
	assert.Equal(t, broken.ID, out.Corrupted[0].ID)
}

func TestSessionGetTakesServerQualifiedIDs(t *testing.T) {
	f := newFleetFixture(t)

	got, err := f.h.SessionGet(t.Context(), sessions.SessionGetIn{SessionID: "box:" + f.boxed.ID})
	require.NoError(t, err)
	assert.Equal(t, "box", got.Server)
	assert.Equal(t, f.boxed.ID, got.ID)

	got, err = f.h.SessionGet(t.Context(), sessions.SessionGetIn{SessionID: f.boxed.ID, Server: "box"})
	require.NoError(t, err)
	assert.Equal(t, "box", got.Server)

	got, err = f.h.SessionGet(t.Context(), sessions.SessionGetIn{SessionID: f.local.ID})
	require.NoError(t, err)
	assert.Equal(t, "local", got.Server)
	assert.Equal(t, "agent."+f.local.ID+".inbox", got.Inbox)

	_, err = f.h.SessionGet(t.Context(), sessions.SessionGetIn{SessionID: "box:missing"})
	assert.Equal(t, ops.KindNotFound, kindOf(t, err))
}

func TestSessionTagsUpdateOnAServer(t *testing.T) {
	f := newFleetFixture(t)

	got, err := f.h.SessionTagsUpdate(t.Context(), sessions.SessionTagsUpdateIn{
		SessionID: "box:" + f.boxed.ID, Add: []string{"claimed"}, Remove: []string{"auto"},
	})
	require.NoError(t, err)
	assert.Equal(t, "box", got.Server)
	assert.Equal(t, []string{"claimed"}, got.Tags)

	again, err := f.h.SessionGet(t.Context(), sessions.SessionGetIn{SessionID: "box:" + f.boxed.ID})
	require.NoError(t, err)
	assert.Equal(t, []string{"claimed"}, again.Tags)
}

func TestSessionUpdate(t *testing.T) {
	f := newFleetFixture(t)

	got, err := f.h.SessionUpdate(t.Context(), sessions.SessionUpdateIn{
		SessionID: "box:" + f.boxed.ID, Group: "backend", ClearTags: true, AddTag: []string{"a", "b"}, RemoveTag: []string{"A"},
	})
	require.NoError(t, err)
	assert.Equal(t, "backend", got.Group)
	assert.Equal(t, []string{"b"}, got.Tags)

	_, err = f.h.SessionUpdate(t.Context(), sessions.SessionUpdateIn{SessionID: f.local.ID, AddTag: []string{"has,comma"}})
	assert.Equal(t, ops.KindInvalid, kindOf(t, err))
}

func TestSessionDeleteForced(t *testing.T) {
	f := newFleetFixture(t)

	got, err := f.h.SessionDelete(t.Context(), sessions.SessionDeleteIn{SessionID: "box:" + f.boxed.ID, Force: true})
	require.NoError(t, err)
	assert.Equal(t, sessions.SessionDeletedOut{Server: "box", ID: f.boxed.ID, Deleted: true}, got)

	_, err = f.h.SessionGet(t.Context(), sessions.SessionGetIn{SessionID: "box:" + f.boxed.ID})
	assert.Equal(t, ops.KindNotFound, kindOf(t, err))
}

func TestSessionCreateRefusals(t *testing.T) {
	f := newFleetFixture(t)
	dir := t.TempDir()

	_, err := f.h.SessionCreate(t.Context(), sessions.SessionCreateIn{Caller: dir, Name: "x", Remote: "https://github.com/acme/site", Source: "relative"})
	assert.Equal(t, ops.KindInvalid, kindOf(t, err))

	_, err = f.h.SessionCreate(t.Context(), sessions.SessionCreateIn{Caller: dir, Name: "x", Server: "box", Remote: "https://github.com/acme/site", Source: dir})
	assert.Equal(t, ops.KindInvalid, kindOf(t, err), "a server cannot read a source on this machine")

	_, err = f.h.SessionCreate(t.Context(), sessions.SessionCreateIn{Caller: dir, Name: "x", Remote: "https://github.com/acme/site", Agent: "nope"})
	assert.Equal(t, ops.KindInvalid, kindOf(t, err))
}

func TestSessionCreateOnAServer(t *testing.T) {
	f := newFleetFixture(t)

	got, err := f.h.SessionCreate(t.Context(), sessions.SessionCreateIn{Caller: t.TempDir(), Name: "made here", Server: "box", Remote: "https://github.com/acme/site", Tags: []string{"auto"}})
	require.NoError(t, err)
	assert.Equal(t, "box", got.Server)
	assert.Equal(t, []string{"auto"}, got.Tags)

	listed, err := f.h.SessionList(t.Context(), sessions.SessionListIn{Server: "box"})
	require.NoError(t, err)
	assert.Contains(t, refs(listed), "box:"+got.ID)
}

func TestSessionRecycleOutMarshalsOneShape(t *testing.T) {
	deleted, err := json.Marshal(sessions.SessionRecycleOut{Deleted: &sessions.SessionDeletedOut{Server: "box", ID: "a1", Deleted: true}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"server":"box","id":"a1","deleted":true}`, string(deleted))

	kept, err := json.Marshal(sessions.SessionRecycleOut{Session: &sessions.SessionOut{Server: "local", ID: "a1", Tags: []string{}}})
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(kept, &fields))
	assert.Equal(t, "a1", fields["id"])
	assert.NotContains(t, fields, "deleted")
}

func TestSessionRecycleAdvertisesTheSchemaOfBothShapes(t *testing.T) {
	f := newFixture(t)
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	for _, op := range f.h.Ops() {
		if op.Spec.Info().Name == sessions.SessionRecycle.Name {
			op.Mount(srv, func(err error) error { return err })
		}
	}
	ct, st := mcp.NewInMemoryTransports()
	_, err := srv.Connect(t.Context(), st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "test"}, nil).Connect(t.Context(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	listed, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, listed.Tools, 1)
	raw, err := json.Marshal(listed.Tools[0].OutputSchema)
	require.NoError(t, err)
	var schema jsonschema.Schema
	require.NoError(t, json.Unmarshal(raw, &schema))
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	now := time.Now()
	for name, out := range map[string]sessions.SessionRecycleOut{
		"kept": {Session: &sessions.SessionOut{
			Server: "local", ID: "a1", Name: "worker", Tags: []string{}, CreatedAt: now, UpdatedAt: now,
		}},
		"deleted": {Deleted: &sessions.SessionDeletedOut{Server: "box", ID: "a1", Deleted: true}},
	} {
		encoded, err := json.Marshal(out)
		require.NoError(t, err)
		var value map[string]any
		require.NoError(t, json.Unmarshal(encoded, &value))
		assert.NoError(t, resolved.Validate(value), name)
	}
}

func TestSessionOpsWithoutAFleetReachThisEngine(t *testing.T) {
	f := newFixture(t)

	out, err := f.h.SessionList(t.Context(), sessions.SessionListIn{})
	require.NoError(t, err)
	assert.Equal(t, []string{"local:" + f.sess.ID}, refs(out))

	_, err = f.h.SessionGet(t.Context(), sessions.SessionGetIn{SessionID: "box:" + f.sess.ID})
	assert.Equal(t, ops.KindNotFound, kindOf(t, err))
}
