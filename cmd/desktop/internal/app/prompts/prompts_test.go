package prompts

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/mcpcatalog"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

func testEnv() Env {
	return Env{
		FlowsDir:           "/home/u/.config/hive/desktop/flows",
		ActionsPath:        "/home/u/.config/hive/desktop/actions.yml",
		SettingsPath:       "/home/u/.config/hive/desktop/settings.yaml",
		WebhookBaseURL:     "http://127.0.0.1:24917/hooks",
		WebhookEnabled:     true,
		MCPEndpoint:        "http://127.0.0.1:24917/mcp",
		MCPEnabled:         true,
		AgentWorkspacesDir: "/home/u/.config/hive/desktop/workspaces",
	}
}

func testInput() Input {
	return Input{WebhookPath: "ci-alerts", WebhookSample: `{"event":"deploy"}`}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	svc, err := New(testEnv())
	require.NoError(t, err)
	return svc
}

// TestEveryDefinitionRenders is the guard on the registry: a definition whose
// template is missing or malformed would otherwise silently vanish from the
// settings page (Catalog skips failures so one bad entry cannot empty it).
func TestEveryDefinitionRenders(t *testing.T) {
	svc := newTestService(t)
	for _, id := range IDs() {
		prompt, err := svc.Render(id, testInput())
		require.NoErrorf(t, err, "prompt %q", id)
		assert.Equalf(t, id, prompt.ID, "prompt %q", id)
		assert.NotEmptyf(t, prompt.Title, "prompt %q has no title", id)
		assert.NotEmptyf(t, prompt.Description, "prompt %q has no description", id)
		assert.NotEmptyf(t, strings.TrimSpace(prompt.Text), "prompt %q rendered empty", id)
		// An unresolved template action leaks as "<no value>" rather than failing.
		assert.NotContainsf(t, prompt.Text, "<no value>", "prompt %q has an unresolved template field", id)
	}
}

// The frame names what started the chat, carries the user's prompt through
// untouched, and gives the exact command that ends the session: two
// environment variables and no JSON, so there is nothing for the agent to
// quote. Without an end URL the closing instruction is absent rather than
// pointing at nothing.
func TestScheduledRunFramesThePrompt(t *testing.T) {
	text, err := ScheduledRun(ScheduledRunData{
		ScheduleName: "Weekly summary", WorkspaceName: "Product",
		Prompt: "Summarize the week since 2026-08-28.\n- include open PRs", CanEnd: true,
	})
	require.NoError(t, err)
	assert.Contains(t, text, `scheduled task "Weekly summary" in the "Product" workspace`)
	assert.Contains(t, text, "Summarize the week since 2026-08-28.\n- include open PRs")
	assert.Contains(t, text, "you MUST end this session")
	assert.Contains(t, text, `curl -fsS -X POST "$HIVE_AGENT_SESSION_END_URL" -H "Authorization: Bearer $HIVE_AGENT_SESSION_TOKEN"`)
	assert.NotContains(t, text, "<no value>")

	quiet, err := ScheduledRun(ScheduledRunData{ScheduleName: "w", WorkspaceName: "p", Prompt: "go"})
	require.NoError(t, err)
	assert.Contains(t, quiet, "go")
	assert.NotContains(t, quiet, "end this session")
	assert.NotContains(t, quiet, "curl")
}

func TestFirstRunNamesTheDefaultProfile(t *testing.T) {
	text, err := FirstRun(FirstRunData{DefaultProfile: "Default"})
	require.NoError(t, err)
	assert.Contains(t, text, `A profile named "Default" already exists`)
	assert.Contains(t, text, "interview")
	assert.Contains(t, text, "ask before you create or change anything")
	assert.NotContains(t, text, "<no value>")
}

func TestRenderRejectsUnknownID(t *testing.T) {
	_, err := newTestService(t).Render("not-a-prompt", testInput())
	require.Error(t, err)
}

// TestFlowsPromptCoversEveryNodeType is the acceptance criterion that adding a
// node type extends the prompt with no prose edit: the prompt is asserted
// against the flow registry, not against a fixed list.
func TestFlowsPromptCoversEveryNodeType(t *testing.T) {
	prompt, err := newTestService(t).Render("flows", testInput())
	require.NoError(t, err)

	for _, nodeType := range flow.NodeTypes() {
		assert.Containsf(t, prompt.Text, "`"+nodeType+"`", "flows prompt omits node type %q", nodeType)

		doc, err := flow.NodeDoc(nodeType)
		require.NoError(t, err)
		// The doc's own heading is a stable substring of its content.
		heading := strings.TrimSpace(strings.SplitN(strings.TrimSpace(doc), "\n", 2)[0])
		assert.Containsf(t, prompt.Text, heading, "flows prompt omits the docs for %q", nodeType)
	}

	for _, category := range flow.CategoryOrder {
		assert.Containsf(t, prompt.Text, string(category), "flows prompt omits category %q", category)
	}
}

// TestFlowsPromptEmbedsCanonicalExample ties the prompt to the fixture the
// loader test validates, so the example cannot drift into something invalid.
func TestFlowsPromptEmbedsCanonicalExample(t *testing.T) {
	prompt, err := newTestService(t).Render("flows", testInput())
	require.NoError(t, err)
	assert.Contains(t, prompt.Text, strings.TrimSpace(flow.WorkedExampleYAML))
}

// TestActionsPromptCoversEveryActionType mirrors the flows assertion for the
// action type registry.
func TestActionsPromptCoversEveryActionType(t *testing.T) {
	prompt, err := newTestService(t).Render("actions", testInput())
	require.NoError(t, err)

	for _, actionType := range actions.Types() {
		assert.Containsf(t, prompt.Text, "`"+actionType+"`", "actions prompt omits action type %q", actionType)

		doc, err := actions.Doc(actionType)
		require.NoError(t, err)
		heading := strings.TrimSpace(strings.SplitN(strings.TrimSpace(doc), "\n", 2)[0])
		assert.Containsf(t, prompt.Text, heading, "actions prompt omits the docs for %q", actionType)
	}
	assert.Contains(t, prompt.Text, strings.TrimSpace(actions.ExampleYAML()))
}

// TestAgentWorkspacesPromptCoversEveryMCPType mirrors the actions assertion
// for the shipped MCP catalogue: the prompt↔catalogue bijection, so "a new
// shipped MCP extends the prompt with no prose edit" is a claim something
// checks.
func TestAgentWorkspacesPromptCoversEveryMCPType(t *testing.T) {
	prompt, err := newTestService(t).Render("agent-workspaces", testInput())
	require.NoError(t, err)

	for _, mcpType := range mcpcatalog.Types() {
		assert.Containsf(t, prompt.Text, "`"+mcpType+"`", "agent-workspaces prompt omits MCP type %q", mcpType)

		doc, err := mcpcatalog.Doc(mcpType)
		require.NoError(t, err)
		heading := strings.TrimSpace(strings.SplitN(strings.TrimSpace(doc), "\n", 2)[0])
		assert.Containsf(t, prompt.Text, heading, "agent-workspaces prompt omits the docs for %q", mcpType)
	}
}

// TestPromptsCarryInstallPaths is the point of rendering server-side: a copied
// prompt names the file on this machine, not a placeholder.
func TestPromptsCarryInstallPaths(t *testing.T) {
	env := testEnv()
	svc := newTestService(t)

	for id, want := range map[string]string{
		"flows":            env.FlowsDir,
		"actions":          env.ActionsPath,
		"settings":         env.SettingsPath,
		"webhook-sources":  env.WebhookBaseURL,
		"mcp":              env.MCPEndpoint,
		"agent-workspaces": env.AgentWorkspacesDir,
	} {
		prompt, err := svc.Render(id, testInput())
		require.NoErrorf(t, err, "prompt %q", id)
		assert.Containsf(t, prompt.Text, want, "prompt %q does not name %q", id, want)
		assert.NotEmptyf(t, prompt.Target, "prompt %q has no target", id)
	}
}

func TestWebhookSourcesPromptFlagsDisabledListener(t *testing.T) {
	env := testEnv()
	env.WebhookEnabled = false
	svc, err := New(env)
	require.NoError(t, err)

	prompt, err := svc.Render("webhook-sources", testInput())
	require.NoError(t, err)
	assert.Contains(t, prompt.Text, "disabled")

	enabled, err := newTestService(t).Render("webhook-sources", testInput())
	require.NoError(t, err)
	assert.NotContains(t, enabled.Text, "currently **disabled**")
}

func TestWebhookTransformPromptUsesCapturedSample(t *testing.T) {
	svc := newTestService(t)

	withSample, err := svc.Render("webhook-transform", testInput())
	require.NoError(t, err)
	assert.Contains(t, withSample.Text, `{"event":"deploy"}`)
	assert.Contains(t, withSample.Text, `"ci-alerts"`)
	assert.NotContains(t, withSample.Text, "<paste a sample payload here>")

	withoutSample, err := svc.Render("webhook-transform", Input{WebhookPath: "ci"})
	require.NoError(t, err)
	assert.Contains(t, withoutSample.Text, "<paste a sample payload here>")

	_, err = svc.Render("webhook-transform", Input{})
	require.Error(t, err, "a node-scoped prompt with no node is a caller bug")
}

// TestMCPPromptPointsAtLiveServer — the prompt's job is to send the agent to
// the live endpoint and let the server describe its own tools, and to flag a
// disabled server rather than pointing at a dead port.
func TestMCPPromptPointsAtLiveServer(t *testing.T) {
	prompt, err := newTestService(t).Render("mcp", testInput())
	require.NoError(t, err)
	assert.Contains(t, prompt.Text, testEnv().MCPEndpoint)
	assert.Contains(t, prompt.Text, "tools/list")
	assert.NotContains(t, prompt.Text, "**disabled**")

	env := testEnv()
	env.MCPEnabled = false
	svc, err := New(env)
	require.NoError(t, err)
	disabled, err := svc.Render("mcp", testInput())
	require.NoError(t, err)
	assert.Contains(t, disabled.Text, "**disabled**")
}

// TestCatalogListsOnlyContextFreePrompts — the settings page renders whatever
// Catalog reports, so a prompt needing instance data must not appear there.
func TestCatalogListsOnlyContextFreePrompts(t *testing.T) {
	catalog := newTestService(t).Catalog(Input{})
	require.NotEmpty(t, catalog)

	ids := make([]string, len(catalog))
	for i, prompt := range catalog {
		ids[i] = prompt.ID
		assert.NotEmptyf(t, prompt.Text, "catalog entry %q rendered empty", prompt.ID)
	}
	assert.Contains(t, ids, "flows")
	assert.Contains(t, ids, "actions")
	assert.NotContains(t, ids, "webhook-transform")
}

func TestRenderIsDeterministic(t *testing.T) {
	svc := newTestService(t)
	for _, id := range IDs() {
		first, err := svc.Render(id, testInput())
		require.NoError(t, err)
		second, err := svc.Render(id, testInput())
		require.NoError(t, err)
		assert.Equalf(t, first.Text, second.Text, "prompt %q is not deterministic", id)
	}
}

// TestSettingsPromptSchemaParses feeds the settings prompt's own schema block to
// the same strict decoder the app loads settings.yaml with. The block is
// hand-written prose rather than generated from the struct, so it is the one
// prompt that can drift without any other test noticing — and it did: it
// documented a `webhooks:` section that has never existed and said the listener
// defaulted off when it defaults on, so an agent following it wrote a file the
// app rejects.
func TestSettingsPromptSchemaParses(t *testing.T) {
	prompt, err := newTestService(t).Render("settings", testInput())
	require.NoError(t, err)

	schema := fencedBlock(t, prompt.Text, "yaml")

	decoder := yaml.NewDecoder(strings.NewReader(schema))
	decoder.KnownFields(true)
	var cfg settings.Settings
	require.NoError(t, decoder.Decode(&cfg), "the documented schema is not loadable settings.yaml")

	// Documenting a key the app ignores is the same failure in the other
	// direction, so assert the values the block claims are the real defaults.
	defaults := settings.DefaultSettings()
	assert.True(t, cfg.HTTP.Enabled, "schema shows http.enabled: true")
	assert.Equal(t, defaults.HTTP.Enabled, cfg.HTTP.Enabled, "http default drifted from the schema block")
}

// fencedBlock returns the first ```<lang> block in text.
func fencedBlock(t *testing.T, text, lang string) string {
	t.Helper()
	_, after, found := strings.Cut(text, "```"+lang+"\n")
	require.Truef(t, found, "no ```%s block in the prompt", lang)
	body, _, found := strings.Cut(after, "```")
	require.Truef(t, found, "unterminated ```%s block", lang)
	return body
}
