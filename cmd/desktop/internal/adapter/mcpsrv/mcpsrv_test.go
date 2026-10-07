package mcpsrv_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/adapter/mcpsrv"
	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/webhook"
)

var update = flag.Bool("update", false, "rewrite golden files")

// testSession builds the app over a fresh config root and drives the real MCP
// server through a real MCP client over the SDK's in-memory transport pair.
// The tools are what is under test, not the HTTP framing, and an in-memory
// client exercises the same registration, schema inference and input
// validation a network client would.
//
// seedConfig, when given, writes into the config dir before the app starts:
// actions.yml is read eagerly at startup and afterwards only by an fsnotify
// watcher, so a bad-hand-edit case has to be on disk first to be observed
// deterministically.
func testSession(t *testing.T, seedConfig ...func(t *testing.T, configDir string)) (*app.App, *mcp.ClientSession) {
	t.Helper()
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	t.Setenv(settings.EnvDataDir, filepath.Join(root, "data"))
	t.Setenv("HIVE_CONFIG", filepath.Join(root, "hive.yaml"))
	t.Setenv(settings.EnvConfigDir, configDir)
	t.Setenv(settings.EnvMockMode, "feed")

	for _, seed := range seedConfig {
		require.NoError(t, os.MkdirAll(configDir, 0o755))
		seed(t, configDir)
	}

	core, err := app.New(t.Context(), app.Config{
		Settings: settings.DefaultSettings(),
		MockMode: settings.MockMode(),
		Logger:   zerolog.Nop(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })

	// Server() registers the whole tool table, and the SDK panics on a tool
	// whose input or output type it cannot infer a schema for — so every test
	// through this harness is also the guard against a bad struct tag.
	srv := mcpsrv.New(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}).Server()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	_, err = srv.Connect(t.Context(), serverTransport, nil)
	require.NoError(t, err)

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).
		Connect(t.Context(), clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	return core, session
}

func seedItem(t *testing.T, core *app.App, profile, external, payload string) int64 {
	t.Helper()
	item, err := stores.NewSeed(core.PipelineDB()).InboxItem(t.Context(), stores.InboxItem{
		ProfileID: profile, SourceKind: "github", SourceScope: "s", ExternalID: external,
		Payload: []byte(payload), Lifecycle: "active",
	})
	require.NoError(t, err)
	return item.ID
}

// call invokes a tool and decodes its structured answer into out. It fails the
// test when the tool reported an error, so a test body reads as the happy path.
func call(t *testing.T, session *mcp.ClientSession, name string, args any, out any) {
	t.Helper()
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.False(t, res.IsError, "tool %s reported an error: %s", name, textOf(res))
	if out == nil {
		return
	}
	require.NotNil(t, res.StructuredContent, "tool %s returned no structured content", name)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, out))
}

// callErr invokes a tool expecting a tool error, and returns its text. A tool
// error is a result with IsError set, never a transport failure — that
// distinction is the adapter's error contract.
func callErr(t *testing.T, session *mcp.ClientSession, name string, args any) string {
	t.Helper()
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err, "a tool failure must not fail the call")
	require.True(t, res.IsError, "tool %s unexpectedly succeeded", name)
	return textOf(res)
}

func textOf(res *mcp.CallToolResult) string {
	var out strings.Builder
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			out.WriteString(text.Text)
		}
	}
	return out.String()
}

func TestToolsListDeclaresEveryToolWithAnObjectInputSchema(t *testing.T) {
	_, session := testSession(t)

	res, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)

	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		assert.NotEmpty(t, tool.Description, "tool %s has no description", tool.Name)

		// The input schema is what a model calls through, so it must be present
		// and an object on every tool — including the ones taking no arguments.
		require.NotNil(t, tool.InputSchema, "tool %s has no input schema", tool.Name)
		schema, ok := tool.InputSchema.(map[string]any)
		require.True(t, ok, "tool %s input schema is not an object", tool.Name)
		assert.Equal(t, "object", schema["type"], "tool %s input schema is not type object", tool.Name)
	}

	assert.ElementsMatch(t, []string{
		"get_status", "list_profiles", "get_flow", "list_feeds", "list_inbox",
		"list_inbox_item_events", "list_item_sessions", "list_actions", "refresh_sources",
		"list_action_runs", "get_action_run",
		"create_profile", "delete_profile",
		"get_profile_image", "set_profile_image", "clear_profile_image",
		"get_node_image", "set_node_image", "clear_node_image",
		"execute_flow",
		"list_workspaces", "list_schedules", "put_schedule", "remove_schedule",
		"preview_schedule", "schedule_runs",
	}, names)
}

func TestToolsListMatchesGolden(t *testing.T) {
	_, session := testSession(t)

	res, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	sort.Slice(res.Tools, func(i, j int) bool { return res.Tools[i].Name < res.Tools[j].Name })
	assertGoldenJSON(t, "tools_list.json", res.Tools)
}

func TestInboxToolsMatchGolden(t *testing.T) {
	core, session := testSession(t)
	require.NoError(t, core.Flows.Save(t.Context(), webhookFlow()))

	seed := stores.NewSeed(core.PipelineDB())
	item, err := seed.InboxItem(t.Context(), stores.InboxItem{
		ProfileID: "hooks", SourceKind: "webhook", SourceScope: "ci", ExternalID: "golden-1",
		Title: "Golden item", URL: "https://example.test/items/golden-1", Payload: []byte(`{"number":1}`),
		Unread: true, Lifecycle: "active", FirstSeenAt: 1_700_000_000_000, LastEventAt: 1_700_000_001_000,
	})
	require.NoError(t, err)
	require.NoError(t, core.Stores.FeedClaims.Upsert(t.Context(), models.FeedClaim{
		ProfileID: "hooks", FeedID: "hooks/inbox", ItemID: item.ID, SourceID: "source:hooks/hook",
	}))
	_, err = seed.InboxEvent(t.Context(), stores.InboxEvent{
		ItemID: item.ID, Kind: "updated", Transition: "none", Attention: "activity", Summary: "Golden event",
		Detail: []byte(`{"changed":"title"}`), CreatedAt: 1_700_000_002_000,
	})
	require.NoError(t, err)

	assertGoldenToolResponse(t, session, "list_feeds", map[string]any{"profile": "hooks"})
	assertGoldenToolResponse(t, session, "list_inbox", map[string]any{"profile": "hooks", "detail": "full"})
	assertGoldenToolResponse(t, session, "list_inbox_item_events", map[string]any{"itemId": item.ID, "detail": "full"})
	assertGoldenToolResponse(t, session, "list_item_sessions", map[string]any{"itemId": item.ID})
}

func assertGoldenToolResponse(t *testing.T, session *mcp.ClientSession, name string, args any) {
	t.Helper()
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.False(t, res.IsError, "tool %s reported an error: %s", name, textOf(res))
	require.NotNil(t, res.StructuredContent, "tool %s returned no structured content", name)
	assertGoldenJSON(t, "inbox_"+name+".json", res.StructuredContent)
}

func assertGoldenJSON(t *testing.T, name string, value any) {
	t.Helper()
	got, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')

	path := filepath.Join("testdata", name)
	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, got, 0o600))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run go test ./internal/adapter/mcpsrv/ -run %s -update", t.Name())
	assert.Equal(t, string(want), string(got))
}

func TestGetStatusReportsTheBuildAndWebhookListener(t *testing.T) {
	_, session := testSession(t)

	var got struct {
		Version string `json:"version"`
		Webhook struct {
			PathPrefix string `json:"pathPrefix"`
		} `json:"webhook"`
	}
	call(t, session, "get_status", struct{}{}, &got)

	assert.Equal(t, "test", got.Version)
	assert.NotEmpty(t, got.Webhook.PathPrefix)
}

func TestProfileLifecycle(t *testing.T) {
	_, session := testSession(t)

	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	call(t, session, "create_profile", map[string]any{"name": "Work"}, &created)
	require.NotEmpty(t, created.ID)
	assert.Equal(t, "Work", created.Name)

	var listed struct {
		Profiles []struct {
			ID string `json:"id"`
		} `json:"profiles"`
	}
	call(t, session, "list_profiles", struct{}{}, &listed)
	ids := make([]string, 0, len(listed.Profiles))
	for _, p := range listed.Profiles {
		ids = append(ids, p.ID)
	}
	assert.Contains(t, ids, created.ID)

	call(t, session, "delete_profile", map[string]any{"profileId": created.ID}, nil)

	call(t, session, "list_profiles", struct{}{}, &listed)
	ids = ids[:0]
	for _, p := range listed.Profiles {
		ids = append(ids, p.ID)
	}
	assert.NotContains(t, ids, created.ID)
}

// Nothing else on the surface reports a node id, and execute_flow's nodeId and
// the node-image tools all require one — so without this the headline tool
// cannot be driven from the server alone.
func TestGetFlowReportsNodeIDsAndWires(t *testing.T) {
	core, session := testSession(t)
	require.NoError(t, core.Flows.Save(t.Context(), webhookFlow()))

	var got struct {
		ID    string `json:"id"`
		Valid bool   `json:"valid"`
		Nodes []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Path string `json:"path"`
		} `json:"nodes"`
		Wires []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"wires"`
	}
	call(t, session, "get_flow", map[string]any{"profileId": "hooks"}, &got)

	assert.Equal(t, "hooks", got.ID)
	assert.True(t, got.Valid)
	require.Len(t, got.Nodes, 2)
	assert.Equal(t, "hook", got.Nodes[0].ID)
	assert.Equal(t, "sources.webhook", got.Nodes[0].Type)
	assert.Equal(t, "ci", got.Nodes[0].Path, "a node's own config is flattened onto it, as it is on disk")
	require.Len(t, got.Wires, 1)
	assert.Equal(t, "hook", got.Wires[0].From)
	assert.Equal(t, "inbox", got.Wires[0].To)
}

// The graph you cannot run is the one you most need to read, so a profile whose
// file does not parse answers with its error rather than not_found.
func TestGetFlowReportsAProfileThatDoesNotParse(t *testing.T) {
	_, session := testSession(t, func(t *testing.T, configDir string) {
		t.Helper()
		flowsDir := filepath.Join(configDir, "flows")
		require.NoError(t, os.MkdirAll(flowsDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(flowsDir, "broken.yaml"), []byte("version: 1\nnodes: [\n"), 0o600))
	})

	var got struct {
		ID    string `json:"id"`
		Valid bool   `json:"valid"`
		Error string `json:"error"`
		Nodes []any  `json:"nodes"`
	}
	call(t, session, "get_flow", map[string]any{"profileId": "broken"}, &got)

	assert.False(t, got.Valid)
	assert.NotEmpty(t, got.Error, "the load error is the whole point of answering at all")
	assert.Empty(t, got.Nodes)
}

func TestGetFlowReportsAnUnknownProfileAsNotFound(t *testing.T) {
	_, session := testSession(t)

	assert.Contains(t, callErr(t, session, "get_flow", map[string]any{"profileId": "nope"}),
		string(app.KindNotFound))
}

func TestListInboxReturnsSeededItemsWithTheirFeed(t *testing.T) {
	core, session := testSession(t)
	seedItem(t, core, "hive", "ext-1", `{"n":1}`)

	var got struct {
		Items []struct {
			ExternalID string          `json:"externalId"`
			FeedID     string          `json:"feedId"`
			Payload    json.RawMessage `json:"payload"`
		} `json:"items"`
	}
	call(t, session, "list_inbox", map[string]any{"profile": "hive", "detail": "full"}, &got)

	require.Len(t, got.Items, 1)
	assert.Equal(t, "ext-1", got.Items[0].ExternalID)
	assert.JSONEq(t, `{"n":1}`, string(got.Items[0].Payload))
}

// A payload is as large as the source made it and a listing repeats it per
// item, so the level that is safe against a real feed has to be the default —
// a tool that answers happily in a test and blows a client's limit against
// production data is a tool nobody can rely on.
func TestListInboxOmitsPayloadsByDefault(t *testing.T) {
	core, session := testSession(t)
	seedItem(t, core, "hive", "ext-1", `{"n":1}`)

	var got struct {
		Detail string `json:"detail"`
		Items  []struct {
			ExternalID string           `json:"externalId"`
			Title      string           `json:"title"`
			Lifecycle  string           `json:"lifecycle"`
			Payload    *json.RawMessage `json:"payload"`
		} `json:"items"`
	}
	call(t, session, "list_inbox", map[string]any{"profile": "hive"}, &got)

	assert.Equal(t, "summary", got.Detail, "the level is echoed so an omitted payload is never read as an absent one")
	require.Len(t, got.Items, 1)
	assert.Nil(t, got.Items[0].Payload)
	assert.Equal(t, "ext-1", got.Items[0].ExternalID, "everything the app itself names still comes back")
	assert.NotEmpty(t, got.Items[0].Lifecycle)
}

func TestListInboxItemEventsOmitsEventDetailByDefault(t *testing.T) {
	core, session := testSession(t)
	id := seedItem(t, core, "hive", "ext-1", `{}`)

	var got struct {
		Detail string `json:"detail"`
	}
	call(t, session, "list_inbox_item_events", map[string]any{"itemId": id}, &got)
	assert.Equal(t, "summary", got.Detail)

	assert.Contains(t, callErr(t, session, "list_inbox_item_events", map[string]any{"itemId": id, "detail": "emitted"}),
		string(app.KindInvalid), "a read has no middle rung — the payload is there or it is not")
}

// A feed exists because a node declares it, not because something has landed in
// it. Reporting only the feeds with rows makes a real-but-empty feed, a
// nonexistent one and a typo the same answer.
func TestListFeedsIncludesADeclaredFeedNothingHasLandedIn(t *testing.T) {
	core, session := testSession(t)
	require.NoError(t, core.Flows.Save(t.Context(), webhookFlow()))

	var got struct {
		Feeds []struct {
			ID       string `json:"id"`
			Declared bool   `json:"declared"`
			Name     string `json:"name"`
			Total    int64  `json:"total"`
		} `json:"feeds"`
	}
	call(t, session, "list_feeds", map[string]any{"profile": "hooks"}, &got)

	require.Len(t, got.Feeds, 1)
	assert.Equal(t, "hooks/inbox", got.Feeds[0].ID)
	assert.True(t, got.Feeds[0].Declared)
	assert.Equal(t, "Inbox", got.Feeds[0].Name)
	assert.Zero(t, got.Feeds[0].Total)
}

func TestListFeedsReportsAnUnknownProfileAsNotFound(t *testing.T) {
	_, session := testSession(t)

	assert.Contains(t, callErr(t, session, "list_feeds", map[string]any{"profile": "nope"}),
		string(app.KindNotFound))
}

// An empty listing is only explained after the query comes back empty. A
// profile whose flow file is gone but whose inbox rows survived is a state
// worth being able to read, and gating the read on the graph would hide it.
func TestListInboxReadsAProfileWithRowsButNoFlow(t *testing.T) {
	core, session := testSession(t)
	seedItem(t, core, "orphaned", "ext-orphan", `{}`)

	var got struct {
		Items []struct {
			ExternalID string `json:"externalId"`
		} `json:"items"`
	}
	call(t, session, "list_inbox", map[string]any{"profile": "orphaned"}, &got)
	require.Len(t, got.Items, 1)
	assert.Equal(t, "ext-orphan", got.Items[0].ExternalID)
}

func TestListInboxReportsAnUnknownProfileOrFeedAsNotFound(t *testing.T) {
	core, session := testSession(t)
	require.NoError(t, core.Flows.Save(t.Context(), webhookFlow()))

	assert.Contains(t, callErr(t, session, "list_inbox", map[string]any{"profile": "nope"}),
		string(app.KindNotFound), "a typo'd profile is not an empty inbox")
	assert.Contains(t, callErr(t, session, "list_inbox", map[string]any{"profile": "hooks", "feed": "hooks/nope"}),
		string(app.KindNotFound), "a feed the graph does not declare is not an empty feed")
}

// The REST route this replaced resolved an item the same two ways, so the tool
// has to as well — and mock mode has no hive sessions behind it, which is what
// makes the seeded link one hive cannot account for and the read drop it.
func TestListItemSessionsResolvesAnItemAndReconcilesOnRead(t *testing.T) {
	core, session := testSession(t)
	id := seedItem(t, core, "p1", "PR_1", `{}`)
	seedItem(t, core, "p2", "PR_1", `{}`) // same external id, second profile

	assert.Contains(t, callErr(t, session, "list_item_sessions", struct{}{}),
		string(app.KindInvalid), "itemId or externalId is required")
	assert.Contains(t, callErr(t, session, "list_item_sessions", map[string]any{"externalId": "PR_1"}),
		string(app.KindConflict), "one external id matched two items")

	ref, err := core.Stores.InboxItems.RefByID(t.Context(), id)
	require.NoError(t, err)
	require.NoError(t, core.Stores.ItemSessions.Link(t.Context(), "sess-a", ref))

	var got struct {
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
	}
	call(t, session, "list_item_sessions", map[string]any{"itemId": id}, &got)
	assert.Empty(t, got.Sessions, "a link hive cannot account for is dropped rather than reported as a ghost")

	links, err := core.Stores.ItemSessions.List(t.Context(), ref)
	require.NoError(t, err)
	assert.Empty(t, links)
}

func TestListInboxItemEventsReportsAnUnknownItemIDAsNotFound(t *testing.T) {
	_, session := testSession(t)

	assert.Contains(t, callErr(t, session, "list_inbox_item_events", map[string]any{"itemId": 9999}),
		string(app.KindNotFound))
}

func TestListInboxRequiresProfileWhenFeedIsSet(t *testing.T) {
	_, session := testSession(t)

	msg := callErr(t, session, "list_inbox", map[string]any{"feed": "hive/prs"})
	assert.Contains(t, msg, string(app.KindInvalid), "the Kind leads the message so an agent can branch on it")
}

func TestListInboxItemEventsResolvesAnExternalID(t *testing.T) {
	core, session := testSession(t)
	seedItem(t, core, "hive", "ext-1", `{}`)

	var got struct {
		Events []struct {
			ID int64 `json:"id"`
		} `json:"events"`
	}
	call(t, session, "list_inbox_item_events", map[string]any{"externalId": "ext-1"}, &got)
	assert.Empty(t, got.Events, "a freshly inserted item has no lifecycle events yet")
}

func TestListInboxItemEventsReportsAnUnknownExternalIDAsNotFound(t *testing.T) {
	_, session := testSession(t)

	msg := callErr(t, session, "list_inbox_item_events", map[string]any{"externalId": "nope"})
	assert.Contains(t, msg, string(app.KindNotFound))
}

func TestListInboxItemEventsNeedsAnIdentifier(t *testing.T) {
	_, session := testSession(t)

	msg := callErr(t, session, "list_inbox_item_events", struct{}{})
	assert.Contains(t, msg, string(app.KindInvalid))
}

func TestListActionsReportsTheCatalogAndItsLoadStatus(t *testing.T) {
	_, session := testSession(t)

	var got struct {
		Path  string `json:"path"`
		Valid bool   `json:"valid"`
	}
	call(t, session, "list_actions", struct{}{}, &got)

	assert.NotEmpty(t, got.Path, "the catalog reports the file to fix")
	assert.True(t, got.Valid)
}

// A malformed actions.yml keeps the last-good catalog in effect, so without a
// reported error an editor cannot tell "accepted" from "rejected and ignored"
// (issue hay-kot/hive-desktop#111). This is the failure the tool's valid/error pair exists for.
func TestListActionsSurfacesAParseError(t *testing.T) {
	_, session := testSession(t, func(t *testing.T, configDir string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "actions.yml"), []byte(
			"version: 1\nactions:\n  - id: copy-url\n    label: Copy URL\n    type: clipboard\n    txt_template: \"{{ .Payload.url }}\"\n",
		), 0o600))
	})

	var got struct {
		Valid   bool `json:"valid"`
		Error   string
		Actions []struct {
			ID string `json:"id"`
		} `json:"actions"`
	}
	call(t, session, "list_actions", struct{}{}, &got)

	assert.False(t, got.Valid, "an unknown key is a hard error, not a silent drop")
	assert.Contains(t, got.Error, "txt_template", "the error names the offending key")
	assert.Empty(t, got.Actions, "nothing was ever loaded, so the last-good catalog is empty")
}

// Mock mode wires no producer, and the Kind has to reach the agent so it can
// tell "nothing to refresh here" from a transient failure worth retrying.
func TestRefreshSourcesIsUnavailableInMockMode(t *testing.T) {
	_, session := testSession(t)

	msg := callErr(t, session, "refresh_sources", struct{}{})
	assert.Contains(t, msg, string(app.KindUnavailable))
}

// The webhook source's feed mark is the second asset shape (ADR webhook-source-image-marks), keyed
// by content hash rather than by id, and it round-trips through the same
// base64-in/image-out contract the avatar does.
func TestNodeImageLifecycle(t *testing.T) {
	core, session := testSession(t)
	require.NoError(t, core.Flows.Save(t.Context(), webhookFlow()))

	target := map[string]any{"profileId": "hooks", "nodeId": "hook"}

	var set struct {
		ProfileID string `json:"profileId"`
		NodeID    string `json:"nodeId"`
		HasImage  bool   `json:"hasImage"`
	}
	call(t, session, "set_node_image", map[string]any{
		"profileId": "hooks", "nodeId": "hook", "imageBase64": onePixelPNG,
	}, &set)
	assert.Equal(t, "hooks", set.ProfileID)
	assert.Equal(t, "hook", set.NodeID)
	assert.True(t, set.HasImage)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_node_image", Arguments: target})
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(res))
	require.Len(t, res.Content, 1)
	img, ok := res.Content[0].(*mcp.ImageContent)
	require.True(t, ok, "get_node_image must answer with image content")
	decoded, err := png.Decode(bytes.NewReader(img.Data))
	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 128, 128), decoded.Bounds(), "the mark is normalized to 128x128")

	var cleared struct {
		HasImage bool `json:"hasImage"`
	}
	call(t, session, "clear_node_image", target, &cleared)
	assert.False(t, cleared.HasImage)
	assert.Contains(t, callErr(t, session, "get_node_image", target), string(app.KindNotFound))
}

// Handler() runs stateless with JSON responses rather than SSE, which is not
// the SDK's default, so the in-memory tests above would not catch a transport
// that cannot complete a handshake. This drives the real handler over TCP with
// a real streamable client — main.go's path, minus the shared listener.
func TestHandlerServesToolsOverHTTP(t *testing.T) {
	core, _ := testSession(t)
	seedItem(t, core, "hive", "ext-http", `{}`)

	srv := httptest.NewServer(mcpsrv.New(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}).Handler())
	t.Cleanup(srv.Close)

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).
		Connect(t.Context(), &mcp.StreamableClientTransport{
			Endpoint: srv.URL, DisableStandaloneSSE: true,
		}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	tools, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, tools.Tools)

	var got struct {
		Items []struct {
			ExternalID string `json:"externalId"`
		} `json:"items"`
	}
	call(t, session, "list_inbox", map[string]any{"profile": "hive"}, &got)
	require.Len(t, got.Items, 1)
	assert.Equal(t, "ext-http", got.Items[0].ExternalID)
}

// Stateless mode means a request carries its own session, so a bare JSON-RPC
// POST works with no initialize handshake and no session header. The
// desktop-api skill documents exactly this curl recipe for when an MCP client
// is not set up, so it is worth pinning.
func TestRawJSONRPCPostNeedsNoHandshake(t *testing.T) {
	core, _ := testSession(t)
	srv := httptest.NewServer(mcpsrv.New(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}).Handler())
	t.Cleanup(srv.Close)

	post := func(body string) map[string]any {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck // test
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Content-Type"), "application/json",
			"JSONResponse is what keeps this curl-able")
		var out map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		return out
	}

	listed := post(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	result, ok := listed["result"].(map[string]any)
	require.True(t, ok, "tools/list answered %v", listed)
	assert.NotEmpty(t, result["tools"])

	called := post(`{"jsonrpc":"2.0","id":2,"method":"tools/call",` +
		`"params":{"name":"get_status","arguments":{}}}`)
	result, ok = called["result"].(map[string]any)
	require.True(t, ok, "tools/call answered %v", called)
	structured, ok := result["structuredContent"].(map[string]any)
	require.True(t, ok, "the skill reads .result.structuredContent")
	assert.Equal(t, "test", structured["version"])
}

// main.go's real path: MountAPI onto the webhook listener, Start binds one
// loopback port, and the tools answer over TCP on it beside /hooks and
// /api/status. The mount is an exact-match mux entry, so this is also what
// pins the endpoint at exactly PathPrefix.
func TestMountedOnTheSharedLoopbackServer(t *testing.T) {
	root := t.TempDir()
	t.Setenv(settings.EnvDataDir, filepath.Join(root, "data"))
	t.Setenv("HIVE_CONFIG", filepath.Join(root, "hive.yaml"))
	t.Setenv(settings.EnvConfigDir, filepath.Join(root, "config"))
	t.Setenv(settings.EnvMockMode, "feed")
	t.Setenv(settings.EnvHTTPEnabled, "true")
	t.Setenv(settings.EnvHTTPPort, "0")

	cfg, err := settings.NewStore(filepath.Join(root, "config", "settings.yaml")).Effective()
	require.NoError(t, err)
	core, err := app.New(t.Context(), app.Config{Settings: cfg, MockMode: cfg.MockMode(), Logger: zerolog.Nop()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })

	require.True(t, core.MountAPI(mcpsrv.PathPrefix,
		mcpsrv.New(core, zerolog.Nop(), mcpsrv.Options{Version: "test"}).Handler()),
		"the webhook listener exists, so the MCP server mounts")
	seedItem(t, core, "hive", "ext-mounted", `{}`)
	require.NoError(t, core.Start(t.Context()))

	running, port := core.Webhooks.Endpoint(t.Context())
	require.True(t, running)
	endpoint := fmt.Sprintf("http://127.0.0.1:%d%s", port, mcpsrv.PathPrefix)

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).
		Connect(t.Context(), &mcp.StreamableClientTransport{
			Endpoint: endpoint, DisableStandaloneSSE: true,
		}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	var got struct {
		Items []struct {
			ExternalID string `json:"externalId"`
		} `json:"items"`
	}
	call(t, session, "list_inbox", map[string]any{"profile": "hive"}, &got)
	require.Len(t, got.Items, 1)
	assert.Equal(t, "ext-mounted", got.Items[0].ExternalID)

	// The endpoint MCPEndpointAt renders into prompt text has to be the one
	// that answers, or the printed URL points at nothing.
	assert.Equal(t, endpoint, app.MCPEndpointAt(core.Webhooks.Host(), port))
}

func webhookFlow() flow.Flow {
	return flow.Flow{
		ID: "hooks", Name: "Hooks", Enabled: true,
		Nodes: []flow.Node{
			{ID: "hook", Type: "sources.webhook", Config: flow.NewSourceConfig(webhook.Descriptor.Type, &webhook.Config{Path: "ci"})},
			{ID: "inbox", Type: "feed", Name: "Inbox", Config: &flow.FeedConfig{}},
		},
		Wires: []flow.Wire{{From: "hook", To: "inbox"}},
	}
}

// The image tools answer with an image content block rather than a structured
// value, so a model that can see images gets the avatar itself.
func TestProfileImageRoundTripsThroughBase64(t *testing.T) {
	_, session := testSession(t)

	var created struct {
		ID string `json:"id"`
	}
	call(t, session, "create_profile", map[string]any{"name": "Work"}, &created)

	var withImage struct {
		HasImage bool `json:"hasImage"`
	}
	call(t, session, "set_profile_image", map[string]any{
		"profileId": created.ID, "imageBase64": onePixelPNG,
	}, &withImage)
	assert.True(t, withImage.HasImage)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "get_profile_image", Arguments: map[string]any{"profileId": created.ID},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, textOf(res))
	require.Len(t, res.Content, 1)
	img, ok := res.Content[0].(*mcp.ImageContent)
	require.True(t, ok, "get_profile_image must answer with image content")
	assert.Equal(t, "image/png", img.MIMEType)
	assert.NotEmpty(t, img.Data)

	var cleared struct {
		HasImage bool `json:"hasImage"`
	}
	call(t, session, "clear_profile_image", map[string]any{"profileId": created.ID}, &cleared)
	assert.False(t, cleared.HasImage)
}

// delete_profile is the only irreversible tool here, so its answer must never
// be a constant: a mistyped id has to be distinguishable from a deletion.
func TestDeleteProfileReportsAnUnknownProfileAsNotFound(t *testing.T) {
	_, session := testSession(t)

	assert.Contains(t, callErr(t, session, "delete_profile", map[string]any{"profileId": "this-never-existed"}),
		string(app.KindNotFound))
}

// The two failures must not read alike: "has no image" asserts the profile
// exists, so answering it for a mistyped id sends the caller looking for an
// avatar rather than for their typo.
func TestProfileImageDistinguishesAMissingProfileFromAMissingAvatar(t *testing.T) {
	_, session := testSession(t)

	var created struct {
		ID string `json:"id"`
	}
	call(t, session, "create_profile", map[string]any{"name": "Work"}, &created)

	assert.Contains(t, callErr(t, session, "get_profile_image", map[string]any{"profileId": created.ID}),
		"has no image")
	missing := callErr(t, session, "get_profile_image", map[string]any{"profileId": "nope"})
	assert.Contains(t, missing, string(app.KindNotFound))
	assert.NotContains(t, missing, "has no image")
}

// An app.Error's Msg is written as a context prefix, so dropping its wrapped
// cause leaves an agent holding a dangling phrase with no reason in it.
func TestAToolErrorCarriesTheWrappedCause(t *testing.T) {
	_, session := testSession(t)

	msg := callErr(t, session, "clear_profile_image", map[string]any{"profileId": "nope"})
	assert.Contains(t, msg, string(app.KindNotFound), "a missing profile is not an invalid request")
	assert.Contains(t, msg, "not found")
}

// A data URL is accepted because an agent handed one by another tool will
// otherwise forget to strip the prefix.
func TestSetProfileImageAcceptsADataURL(t *testing.T) {
	_, session := testSession(t)

	var created struct {
		ID string `json:"id"`
	}
	call(t, session, "create_profile", map[string]any{"name": "Work"}, &created)

	var got struct {
		HasImage bool `json:"hasImage"`
	}
	call(t, session, "set_profile_image", map[string]any{
		"profileId": created.ID, "imageBase64": "data:image/png;base64," + onePixelPNG,
	}, &got)
	assert.True(t, got.HasImage)
}

func TestSetProfileImageRejectsNonBase64(t *testing.T) {
	_, session := testSession(t)

	var created struct {
		ID string `json:"id"`
	}
	call(t, session, "create_profile", map[string]any{"name": "Work"}, &created)

	msg := callErr(t, session, "set_profile_image", map[string]any{
		"profileId": created.ID, "imageBase64": "not base64!!",
	})
	assert.Contains(t, msg, string(app.KindInvalid))
}

func TestExecuteFlowNeedsExactlyOneFlowSource(t *testing.T) {
	_, session := testSession(t)

	msg := callErr(t, session, "execute_flow", map[string]any{
		"nodeId":   "n1",
		"messages": []map[string]any{{"ID": "1"}},
	})
	assert.Contains(t, msg, "exactly one of flowId, flow, or flowYaml")
}

// A JSON-object payload has to survive the schema and reach the node. It is
// worth asserting because models.Msg carries its payload as json.RawMessage,
// whose inferred schema is an array — passing models.Msg straight through as
// the tool's input type makes every real payload unrepresentable.
func TestExecuteFlowDeliversAnObjectPayloadAndCommitsNothing(t *testing.T) {
	core, session := testSession(t)

	var got struct {
		FlowID string `json:"flowId"`
		Nodes  []struct {
			NodeID   string `json:"nodeId"`
			Received []struct {
				Payload json.RawMessage `json:"Payload"`
			} `json:"received"`
		} `json:"nodes"`
	}
	call(t, session, "execute_flow", map[string]any{
		"flowYaml":       inlineFlowYAML,
		"flowIdOverride": "scratch",
		"nodeId":         "fn",
		"detail":         "full",
		"messages": []map[string]any{{
			"ID": "1", "Topic": "t", "Payload": map[string]any{"a": 1},
		}},
	}, &got)

	assert.Equal(t, "scratch", got.FlowID)
	require.NotEmpty(t, got.Nodes)
	require.NotEmpty(t, got.Nodes[0].Received)
	assert.JSONEq(t, `{"a":1}`, string(got.Nodes[0].Received[0].Payload))

	// A dry run installs nothing: the flow it ran must not appear afterwards.
	for _, st := range core.Flows.Statuses(t.Context()) {
		assert.NotEqual(t, "scratch", st.ID, "a dry run must not install the flow it ran")
	}
}

// A message costs one copy per hop unless the caller says otherwise: every node
// reports it in received and again in emitted, so a payload through a real
// graph comes back many times over. The default drops received, which is
// derivable from the upstream's emitted, and counts drops bodies entirely.
func TestExecuteFlowDetailControlsHowManyCopiesOfAPayloadComeBack(t *testing.T) {
	_, session := testSession(t)

	run := func(t *testing.T, detail string) (nodes []struct {
		NodeID   string `json:"nodeId"`
		In       int    `json:"in"`
		Received *[]any `json:"received"`
		Emitted  *[]any `json:"emitted"`
	}, level string,
	) {
		t.Helper()
		args := map[string]any{
			"flowYaml": inlineFlowYAML, "flowIdOverride": "scratch", "nodeId": "fn",
			"messages": []map[string]any{{"ID": "1", "Topic": "t", "Payload": map[string]any{"a": 1}}},
		}
		if detail != "" {
			args["detail"] = detail
		}
		var got struct {
			Detail string `json:"detail"`
			Nodes  []struct {
				NodeID   string `json:"nodeId"`
				In       int    `json:"in"`
				Received *[]any `json:"received"`
				Emitted  *[]any `json:"emitted"`
			} `json:"nodes"`
		}
		call(t, session, "execute_flow", args, &got)
		require.NotEmpty(t, got.Nodes)
		return got.Nodes, got.Detail
	}

	t.Run("default omits received", func(t *testing.T) {
		nodes, level := run(t, "")
		assert.Equal(t, "emitted", level, "the level is echoed so absent and empty stay distinguishable")
		assert.Nil(t, nodes[0].Received)
		assert.NotNil(t, nodes[0].Emitted)
	})

	t.Run("summary omits both but keeps the counters", func(t *testing.T) {
		nodes, level := run(t, "summary")
		assert.Equal(t, "summary", level)
		assert.Nil(t, nodes[0].Received)
		assert.Nil(t, nodes[0].Emitted)
		assert.Equal(t, 1, nodes[0].In, "what each node did is still reported, only the bodies are gone")
	})

	t.Run("full keeps both", func(t *testing.T) {
		nodes, level := run(t, "full")
		assert.Equal(t, "full", level)
		assert.NotNil(t, nodes[0].Received)
		assert.NotNil(t, nodes[0].Emitted)
	})
}

// A terminal emits nothing, and that has to arrive as [] rather than as an
// absent key: absent is what "this detail level did not report it" means, and
// the run's collections are non-nil precisely so tooling can count them.
func TestExecuteFlowDistinguishesAnEmptyEmissionFromAnOmittedOne(t *testing.T) {
	core, session := testSession(t)
	require.NoError(t, core.Flows.Save(t.Context(), webhookFlow()))

	var got struct {
		Nodes []struct {
			NodeID  string `json:"nodeId"`
			Emitted *[]any `json:"emitted"`
		} `json:"nodes"`
	}
	call(t, session, "execute_flow", map[string]any{
		"flowId": "hooks", "nodeId": "hook",
		"messages": []map[string]any{{"Key": "k", "Payload": map[string]any{"a": 1}}},
	}, &got)

	var terminal *[]any
	for _, node := range got.Nodes {
		if node.NodeID == "inbox" {
			terminal = node.Emitted
		}
	}
	require.NotNil(t, terminal, "the feed terminal reported no emitted key at all")
	assert.Empty(t, *terminal)
}

func TestExecuteFlowRejectsAnUnknownDetailLevel(t *testing.T) {
	_, session := testSession(t)

	msg := callErr(t, session, "execute_flow", map[string]any{
		"flowYaml": inlineFlowYAML, "nodeId": "fn", "detail": "verbose",
		"messages": []map[string]any{{"ID": "1"}},
	})
	assert.Contains(t, msg, string(app.KindInvalid))
}

// An input the schema rejects never reaches the handler — the SDK validates
// against the inferred schema first, which is what "declared capabilities"
// buys at this boundary.
func TestSchemaValidationRejectsAWronglyTypedArgument(t *testing.T) {
	_, session := testSession(t)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "list_inbox", Arguments: map[string]any{"limit": "many"},
	})
	if err == nil {
		require.True(t, res.IsError, "a wrongly typed argument must not be accepted")
		return
	}
	assert.Contains(t, err.Error(), "limit")
}

const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// A single function node the dry run injects into, wired to nothing. It is
// inline so the test exercises the unsaved-edit path, which is the reason
// execute_flow takes a document at all.
const inlineFlowYAML = `version: 1
name: Scratch
enabled: false
nodes:
  - id: fn
    type: function
    outputs: 1
    on_message: "return msg;"
wires: []
`
