package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/agentws"
	"github.com/colonyops/hive/cmd/desktop/internal/app/canvas"
	"github.com/colonyops/hive/cmd/desktop/internal/app/configmigrate"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/execenv"
	"github.com/colonyops/hive/cmd/desktop/internal/app/tmuxcc"
	"github.com/colonyops/hive/pkg/executil"
)

// newTestAgentWorkspacesService builds a service over root with a real
// tmuxcc.Manager pointed at a private tmux server (requireTmux/privateTmux,
// terminals_service_test.go — a session's liveness is a real tmux fact, and a
// faked one would only prove the fake), a real sqlite-backed queries.DB, and a
// real SkillsService (newTestSkillsService). commands stands in for
// agentCommands(hiveCfg) — the caller picks which agent keys are "configured"
// and what they run.
func newTestAgentWorkspacesService(t *testing.T, root string, commands map[string]string) *AgentWorkspacesService {
	t.Helper()
	privateTmux(t)

	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	manager := tmuxcc.NewManager(t.Context(), tmuxcc.ManagerOptions{Logger: zerolog.Nop()})
	t.Cleanup(func() { _ = manager.Stop(context.WithoutCancel(t.Context())) })

	awStore := agentws.NewStore(root)
	require.NoError(t, awStore.Reload())

	return newAgentWorkspacesService(AgentWorkspacesDeps{
		Store:           awStore,
		Terminals:       manager,
		Stores:          stores.New(db, stores.Options{}),
		Skills:          newTestSkillsService(t),
		ProfileCommands: func() map[string]string { return commands },
		MCPBase:         mcpBaseFunc(func(context.Context) string { return testMCPBaseURL }),
		Events:          events.New(zerolog.Nop()),
	})
}

// testMCPBaseURL stands in for this run's loopback base URL, which the
// catalogue joins with each app-hosted entry's RuntimePath.
const testMCPBaseURL = "http://127.0.0.1:24917"

type mcpBaseFunc func(context.Context) string

func (f mcpBaseFunc) MCPBaseURL(ctx context.Context) string { return f(ctx) }

type editorCommandFunc func(context.Context) (string, error)

func (f editorCommandFunc) Editor(ctx context.Context) (string, error) { return f(ctx) }

// liveAgentSessionCount is the test-side equivalent of the service's own
// liveSessionCount, used to assert how many agentws-* tmux sessions a call
// left running.
func liveAgentSessionCount(t *testing.T, svc *AgentWorkspacesService) int {
	t.Helper()
	names, err := svc.terminals.SessionNames(t.Context(), agentSessionPrefix)
	require.NoError(t, err)
	return len(names)
}

func writeAgentWorkspaceManifest(t *testing.T, root, dir, body string) string {
	t.Helper()
	wsDir := filepath.Join(root, dir)
	require.NoError(t, os.MkdirAll(wsDir, 0o700))
	path := filepath.Join(wsDir, "agent-workspace.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return wsDir
}

func writeMCPLibrary(t *testing.T, root, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, "mcps.yaml"), []byte(body), 0o600))
}

func writeSharedSkill(t *testing.T, root, slug, body string) {
	t.Helper()
	dir := filepath.Join(agentws.SharedSkillsDir(root), slug)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o600))
}

// newManifestOnlyService builds the service without a tmux manager, for the
// reads that never launch anything (presets, the agent list, manifest
// validation). newTestAgentWorkspacesService needs a real tmux server and so
// skips outside CI; these assertions do not, and skipping them locally would
// hide the parts of this surface a developer is most likely to break.
func newManifestOnlyService(t *testing.T, root string, profileCommands map[string]string) *AgentWorkspacesService {
	t.Helper()

	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	awStore := agentws.NewStore(root)
	require.NoError(t, awStore.Reload())

	return newAgentWorkspacesService(AgentWorkspacesDeps{
		Store: awStore, Stores: stores.New(db, stores.Options{}), Skills: newTestSkillsService(t),
		ProfileCommands: func() map[string]string { return profileCommands }, MCPBase: mcpBaseFunc(func(context.Context) string { return testMCPBaseURL }),
		Events: events.New(zerolog.Nop()),
	})
}

// resumableCommand is a fake-agent command that distinguishes a resume from a
// fresh launch, the way the shipped claude presets do. Resume support is a
// property of the template now (agentws.SupportsResume renders it both ways
// and compares), so a test about resuming has to write one that branches --
// a bare binary path cannot resume no matter which agent labels it.
func resumableCommand(binary string) string {
	return binary + " {{ if .Resume }}--resume{{ else }}--session-id{{ end }} {{ .SessionID }}"
}

// agentWorkspaceYAML is a current-version manifest whose command: is exactly
// the given launch line. The tests drive a fake binary rather than a real
// agent CLI, and the command is now the manifest's own field, so a launching
// test has to write it there instead of configuring it on the service.
func agentWorkspaceYAML(name, command, extra string) string {
	return fmt.Sprintf("version: %d\nname: %s\ncommand: %s\n%s",
		configmigrate.AgentWorkspaceSet.Current, name, command, extra)
}

func writeSkillPackages(t *testing.T, root, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(root, 0o700))
	require.NoError(t, os.WriteFile(agentws.SkillLibraryPath(root), []byte(body), 0o600))
}

// fakeAgentBinary writes a script that ignores every argument it is invoked
// with and execs body -- standing in for a real agent CLI, which would
// otherwise be required to exercise a live, addressable terminal. Because the
// script never references its own $@, the MCP/session flags a command template
// carries have nowhere to land and cannot break it.
//
// Its name is the agent it stands in for: AgentFor reads the command's first
// word, so the file name decides which probe and classifier a test exercises.
func fakeAgentBinary(t *testing.T, agent, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), agent)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexec "+body+"\n"), 0o755))
	return path
}

func TestOpenResolvesMCPsAndSkills(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeMCPLibrary(t, root, "version: 1\nservers:\n  my-user-mcp:\n    command: \"true\"\n")
	writeSkillPackages(t, root, "version: 1\npackages:\n  hive:\n    include: [\"hive-mcp\"]\n")
	writeAgentWorkspaceManifest(t, root, "demo", "version: 2\nname: Demo\nagent: claude\ncommand: true\n"+
		"mcps:\n  - playwright\n  - my-user-mcp\nskills:\n  - hive\n")

	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	result, err := svc.Open(t.Context(), "demo")
	require.NoError(t, err)
	assert.Empty(t, result.MissingMCPs)
	assert.Equal(t, "demo", result.Workspace.Dir)
	assert.Equal(t, "Demo", result.Workspace.Name)
	assert.ElementsMatch(t, []string{"playwright", "my-user-mcp"}, result.Workspace.MCPs)
	assert.Empty(t, result.Sessions)

	mcpJSON, err := os.ReadFile(filepath.Join(root, "demo", ".mcp.json"))
	require.NoError(t, err)
	assert.Contains(t, string(mcpJSON), "playwright")
	assert.Contains(t, string(mcpJSON), "my-user-mcp")

	codexTOML, err := os.ReadFile(filepath.Join(root, "demo", ".codex", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(codexTOML), "mcp_servers.playwright")
	assert.Contains(t, string(codexTOML), "mcp_servers.my-user-mcp")

	skillFile, err := os.ReadFile(filepath.Join(root, "demo", ".claude", "skills", "hive-mcp", "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(skillFile), "name: hive-mcp")
}

func TestOpenOmitsAndReportsAnUnknownMCP(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "demo", "version: 2\nname: Demo\nagent: claude\ncommand: true\nmcps:\n  - playwright\n  - ghost\n")

	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	result, err := svc.Open(t.Context(), "demo")
	require.NoError(t, err, "an unknown mcp id does not fail the open")
	assert.Equal(t, []string{"ghost"}, result.MissingMCPs)

	mcpJSON, err := os.ReadFile(filepath.Join(root, "demo", ".mcp.json"))
	require.NoError(t, err)
	assert.Contains(t, string(mcpJSON), "playwright")
	assert.NotContains(t, string(mcpJSON), "ghost")
}

func TestOpenRejectsAWorkspaceOutsideTheRoot(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	for _, bad := range []string{"../escape", "/etc/passwd", "sub/dir", ".", ""} {
		_, err := svc.Open(t.Context(), bad)
		require.Error(t, err, "dir %q", bad)
		assert.Equal(t, KindInvalid, KindOf(err), "dir %q", bad)
	}

	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries, "a rejected workspace argument must write nothing")
}

func TestLaunchWorkspaceSessionCarriesPromptIntoDetachedChat(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	capture := filepath.Join(t.TempDir(), "prompt")
	agent := filepath.Join(t.TempDir(), "codex")
	require.NoError(t, os.WriteFile(agent, []byte("#!/bin/sh\nprintf '%s' \"$2\" > "+capture+"\nexec cat\n"), 0o755))
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agent+agentws.PromptTail, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"codex": agent})

	origin := models.ItemRef{ProfileID: "default", SourceKind: "github", SourceScope: "acme/app", ExternalID: "pr:7"}
	outcome, err := svc.LaunchWorkspaceSession(t.Context(), dispatch.LaunchWorkspaceSessionRequest{
		Workspace: "demo", Name: "alert", Prompt: "cluster prod is down", Origins: []models.ItemRef{origin},
	})
	require.NoError(t, err)
	assert.Equal(t, "alert", outcome.Name)
	chats, err := svc.itemLinks.ListChats(t.Context(), origin)
	require.NoError(t, err)
	require.Len(t, chats, 1, "the launch links its chat to the originating item")
	assert.Equal(t, outcome.ID, strconv.FormatInt(chats[0].ChatID, 10))
	assert.NotEmpty(t, outcome.ID)
	assert.NotEmpty(t, outcome.Slug)
	assert.Equal(t, 1, liveAgentSessionCount(t, svc))
	require.Eventually(t, func() bool {
		got, readErr := os.ReadFile(capture)
		return readErr == nil && string(got) == "cluster prod is down"
	}, time.Second, 10*time.Millisecond)
	_, err = os.Stat(filepath.Join(root, "demo", ".mcp.json"))
	require.NoError(t, err, "a launch refreshes generated workspace files")
}

func TestStartSessionUsesTheResolvedWorkspaceSnapshot(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	capture := filepath.Join(t.TempDir(), "prompt")
	agent := filepath.Join(t.TempDir(), "codex")
	require.NoError(t, os.WriteFile(agent, []byte("#!/bin/sh\nprintf '%s' \"$2\" > "+capture+"\nexec cat\n"), 0o755))
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agent, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"codex": agent})

	_, err := svc.StartSession(t.Context(), StartSession{
		Workspace: "demo", Name: "current", Prompt: "do not drop this", Detached: true,
	})
	require.ErrorContains(t, err, "does not pass a prompt")

	resolved := agentws.Workspace{Dir: "demo", Name: "Demo", Command: agent + agentws.PromptTail}
	started, err := svc.startSession(t.Context(), resolved, StartSession{
		Workspace: "demo", Name: "resolved", Prompt: "keep this prompt", Detached: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "demo", started.Workspace)
	require.Eventually(t, func() bool {
		got, readErr := os.ReadFile(capture)
		return readErr == nil && string(got) == "keep this prompt"
	}, time.Second, 10*time.Millisecond)
}

func TestLaunchWorkspaceSessionReportsAnImmediateExit(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agent := fakeAgentBinary(t, "codex", "exit 1")
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agent+agentws.PromptTail, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"codex": agent})

	_, err := svc.LaunchWorkspaceSession(t.Context(), dispatch.LaunchWorkspaceSessionRequest{
		Workspace: "demo", Name: "alert", Prompt: "triage this",
	})
	require.ErrorContains(t, err, "session exited immediately")
	assert.Equal(t, 0, liveAgentSessionCount(t, svc))
	sessions, listErr := svc.Sessions(t.Context(), "demo")
	require.NoError(t, listErr)
	assert.Empty(t, sessions, "a failed detached launch must not leave a dead chat to accumulate on retry")
}

func TestLaunchWorkspaceSessionRefusesPromptlessCommand(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agent := fakeAgentBinary(t, "codex", "cat")
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agent, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"codex": agent})

	_, err := svc.LaunchWorkspaceSession(t.Context(), dispatch.LaunchWorkspaceSessionRequest{
		Workspace: "demo", Name: "alert", Prompt: "do not drop this",
	})
	require.ErrorContains(t, err, "does not pass a prompt")
	sessions, listErr := svc.Sessions(t.Context(), "demo")
	require.NoError(t, listErr)
	assert.Empty(t, sessions)
	assert.Equal(t, 0, liveAgentSessionCount(t, svc))
}

func TestSessionLaunchWorkspacesReportsPromptSupport(t *testing.T) {
	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "plain", agentWorkspaceYAML("Plain", "codex", ""))
	writeAgentWorkspaceManifest(t, root, "prompted", agentWorkspaceYAML("Prompted", "codex"+agentws.PromptTail, ""))
	svc := newManifestOnlyService(t, root, nil)

	assert.Equal(t, []dispatch.SessionLaunchWorkspace{
		{Dir: "plain", Name: "Plain", SupportsPrompt: false},
		{Dir: "prompted", Name: "Prompted", SupportsPrompt: true},
	}, svc.SessionLaunchWorkspaces(t.Context()))
}

func TestResumeFallsBackToFreshLaunch(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agentCmd := fakeAgentBinary(t, "codex", "cat")
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agentCmd, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"codex": agentCmd})

	started, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "s1", Cols: 80, Rows: 24})
	require.NoError(t, err)
	require.Regexp(t, `^agentws-[a-z0-9]{8}$`, started.TerminalID)
	assert.Equal(t, started.TerminalID, started.Slug)
	assert.True(t, started.ResumeAttempted, "codex has no resume form, but a fresh start always ResumeAttempted=true")

	closed, err := svc.CloseSession(t.Context(), started.ID)
	require.NoError(t, err)
	require.True(t, closed)

	resumed, err := svc.ResumeSession(t.Context(), started.ID, 80, 24)
	require.NoError(t, err)
	assert.False(t, resumed.ResumeAttempted, "codex cannot resume, so this launch could not even try")
	assert.NotEmpty(t, resumed.Notice)
	assert.NotEmpty(t, resumed.TerminalID, "a fresh relaunch still produces a live session")
}

func TestResumeReattachesTheSameTerminal(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agentCmd := fakeAgentBinary(t, "claude", "cat")
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agentCmd, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})

	started, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "s1", Cols: 80, Rows: 24})
	require.NoError(t, err)
	require.NotEmpty(t, started.TerminalID)

	resumed, err := svc.ResumeSession(t.Context(), started.ID, 80, 24)
	require.NoError(t, err)
	assert.Equal(t, started.TerminalID, resumed.TerminalID, "a resume within one run reattaches rather than spawning beside it")
	assert.True(t, resumed.ResumeAttempted)
	assert.Positive(t, resumed.Cols, "an attach must report tmux's granted grid, or the pane opens at the wrong size")
	assert.Positive(t, resumed.Rows)
	assert.Equal(t, 1, liveAgentSessionCount(t, svc), "reattaching must not spawn a second terminal")
}

// TestResumeOfANeverMessagedClaudeSessionRelaunchesFresh is the reported bug:
// start a chat, send nothing, kill it, restart it. claude persists a
// conversation file only on the first message, so passing --resume for the
// untouched id would die in the pane on "No conversation found with session
// ID". The relaunch must go fresh instead — and silently, because a fresh
// launch IS the continuation of an empty conversation.
func TestResumeOfANeverMessagedClaudeSessionRelaunchesFresh(t *testing.T) {
	isolateConfig(t)
	claudeCfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claudeCfg)
	require.NoError(t, os.MkdirAll(filepath.Join(claudeCfg, "projects", "-elsewhere"), 0o700))

	root := t.TempDir()
	agentCmd := fakeAgentBinary(t, "claude", "cat")
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", resumableCommand(agentCmd), ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})

	started, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "s1", Cols: 80, Rows: 24})
	require.NoError(t, err)
	before, err := svc.sessions.Get(t.Context(), started.ID)
	require.NoError(t, err)

	closed, err := svc.CloseSession(t.Context(), started.ID)
	require.NoError(t, err)
	require.True(t, closed)

	resumed, err := svc.ResumeSession(t.Context(), started.ID, 80, 24)
	require.NoError(t, err)
	assert.False(t, resumed.ResumeAttempted, "nothing was persisted, so this launch must not pass --resume")
	assert.Empty(t, resumed.Notice, "an empty conversation relaunching fresh is a continuation, not a loss to announce")
	assert.NotEmpty(t, resumed.TerminalID)

	after, err := svc.sessions.Get(t.Context(), started.ID)
	require.NoError(t, err)
	assert.NotEqual(t, before.AgentSessionID, after.AgentSessionID,
		"the fresh launch mints a fresh id — reusing one the agent might hold would wedge on 'already in use'")
}

// The counterpart: once the conversation file exists, a relaunch resumes it
// under the same id.
func TestResumeOfAMessagedClaudeSessionResumesById(t *testing.T) {
	isolateConfig(t)
	claudeCfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claudeCfg)

	root := t.TempDir()
	agentCmd := fakeAgentBinary(t, "claude", "cat")
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", resumableCommand(agentCmd), ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})

	started, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "s1", Cols: 80, Rows: 24})
	require.NoError(t, err)
	rec, err := svc.sessions.Get(t.Context(), started.ID)
	require.NoError(t, err)

	projectDir := filepath.Join(claudeCfg, "projects", "-demo")
	require.NoError(t, os.MkdirAll(projectDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, rec.AgentSessionID+".jsonl"), []byte("{}\n"), 0o600))

	closed, err := svc.CloseSession(t.Context(), started.ID)
	require.NoError(t, err)
	require.True(t, closed)

	resumed, err := svc.ResumeSession(t.Context(), started.ID, 80, 24)
	require.NoError(t, err)
	assert.True(t, resumed.ResumeAttempted)
	assert.Empty(t, resumed.Notice)

	after, err := svc.sessions.Get(t.Context(), started.ID)
	require.NoError(t, err)
	assert.Equal(t, rec.AgentSessionID, after.AgentSessionID, "a real resume keeps addressing the same conversation")
}

func TestCloseSessionEndsTheTerminalAndKeepsTheRecord(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agentCmd := fakeAgentBinary(t, "claude", "cat")
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agentCmd, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})

	started, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "s1", Cols: 80, Rows: 24})
	require.NoError(t, err)
	require.NotEmpty(t, started.TerminalID)

	closed, err := svc.CloseSession(t.Context(), started.ID)
	require.NoError(t, err)
	assert.True(t, closed)

	result, err := svc.Open(t.Context(), "demo")
	require.NoError(t, err)
	require.Len(t, result.Sessions, 1)
	assert.Empty(t, result.Sessions[0].TerminalID, "the PTY is gone")
	assert.Equal(t, "s1", result.Sessions[0].Name, "the record still lists")
}

func TestDeleteEndsLiveTerminals(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agentCmd := fakeAgentBinary(t, "claude", "cat")
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agentCmd, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})

	s1, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "s1", Cols: 80, Rows: 24})
	require.NoError(t, err)
	s2, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "s2", Cols: 80, Rows: 24})
	require.NoError(t, err)
	require.Equal(t, 2, liveAgentSessionCount(t, svc))

	canvases := canvas.NewStore(root)
	_, err = canvases.Upsert("demo", "plan", s1.ID, "", "", canvas.Block{ID: "a", Kind: canvas.KindMarkdown, Body: "x"})
	require.NoError(t, err)

	require.NoError(t, svc.DeleteSession(t.Context(), s1.ID))
	_, err = svc.sessions.Get(t.Context(), s1.ID)
	assert.True(t, stores.IsNotFound(err), "the record is gone too")
	_, ok, err := canvases.Load("demo", "plan")
	require.NoError(t, err)
	assert.True(t, ok, "the canvas outlives the chat that made it")
	assert.Equal(t, 1, liveAgentSessionCount(t, svc))

	require.NoError(t, svc.DeleteWorkspace(t.Context(), "demo"))
	_, err = svc.sessions.Get(t.Context(), s2.ID)
	assert.True(t, stores.IsNotFound(err))
	metas, err := canvases.List("demo")
	require.NoError(t, err)
	assert.Empty(t, metas, "canvases live in the workspace folder, which the delete takes with it")
	assert.Equal(t, 0, liveAgentSessionCount(t, svc), "every live terminal the workspace held is gone")
}

func TestStartSessionReportsAnImmediateExit(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agentCmd := "this-binary-does-not-exist-anywhere-12345"
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agentCmd, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})

	started, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "s1", Cols: 80, Rows: 24})
	require.NoError(t, err, "a command not on PATH is a live shell that exits 127, not an exec error")
	assert.Empty(t, started.TerminalID, "a dead terminal must not be reported as live")
	assert.NotEmpty(t, started.Notice)
	assert.Equal(t, 0, liveAgentSessionCount(t, svc), "tmux already ended the session when its command exited")
}

// Nothing has opened this workspace before, so the launch must generate its
// wiring. The missing agent binary makes the session exit without leaving a
// tmux server behind.
func TestStartFirstRunChatOpensTheHiveWorkspaceOnTheInterview(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agentCmd := "this-binary-does-not-exist-anywhere-12345"
	writeAgentWorkspaceManifest(t, root, agentws.HiveWorkspaceDir, agentWorkspaceYAML("Hive", agentCmd+agentws.PromptTail, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})

	started, err := svc.StartFirstRunChat(t.Context())
	require.NoError(t, err)
	assert.Equal(t, agentws.HiveWorkspaceDir, started.Workspace)
	assert.Equal(t, firstRunChatName, started.Name)
	assert.Empty(t, started.TerminalID, "a detached launch hands back no pane; the route attaches")
	assert.FileExists(t, filepath.Join(root, agentws.HiveWorkspaceDir, ".mcp.json"), "the workspace was generated before the launch")

	_, err = svc.StartFirstRunChat(t.Context())
	require.NoError(t, err, "a second hand-off is another chat, not a conflict")
}

// The startup seed runs only when the root is created, so a root that
// something else made first has no Hive workspace. The hand-off puts it back
// rather than telling the user the workspace the card promised is missing.
func TestStartFirstRunChatSeedsAMissingHiveWorkspace(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "this-binary-does-not-exist-anywhere-12345"})
	_, ok := svc.store.Status(agentws.HiveWorkspaceDir)
	require.False(t, ok)

	started, err := svc.StartFirstRunChat(t.Context())
	require.NoError(t, err)

	assert.Equal(t, agentws.HiveWorkspaceDir, started.Workspace)
	st, ok := svc.store.Status(agentws.HiveWorkspaceDir)
	require.True(t, ok, "the seeded workspace is in the store")
	assert.True(t, st.Valid)
	assert.Equal(t, []string{"hive-desktop", "hive-canvas"}, st.Workspace.MCPs)
	assert.FileExists(t, filepath.Join(root, agentws.HiveWorkspaceDir, "AGENTS.md"))
}

// TestDetachedLaunchThatNeverStartedLeavesNoRecord: the session row is written
// before tmux is asked for anything, so a launch that fails there leaves a
// record for a chat that does not exist. A user-driven launch keeps it -- the
// error is on screen and the row is what they retry from -- but a schedule
// firing every minute against a reached cap or a tmux that is down would add
// one dead row a minute to the sidebar, with nobody watching.
func TestDetachedLaunchThatNeverStartedLeavesNoRecord(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agentCmd := fakeAgentBinary(t, "claude", "cat")
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agentCmd, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})

	// tmux refuses a session name it already holds, which is how a launch is
	// made to fail after its record exists.
	launchBlocked := func(name, terminalID string, detached bool) {
		rec, err := svc.sessions.Create(t.Context(), stores.AgentSessionCreate{
			Workspace: "demo", Name: name, Agent: "claude", TerminalID: terminalID,
		})
		require.NoError(t, err)
		blocker := exec.CommandContext(t.Context(), "tmux", "new-session", "-d", "-s", sessionName(rec))
		blocker.Env = executil.WithoutEnv(nil, "TMUX", "TMUX_PANE")
		require.NoError(t, blocker.Run())

		_, err = svc.launchTerminal(t.Context(), rec, terminalLaunch{dir: root, line: agentCmd, detached: detached})
		require.Error(t, err)
	}

	launchBlocked("scheduled", "blocked1", true)
	records, err := svc.sessions.List(t.Context(), "demo")
	require.NoError(t, err)
	assert.Empty(t, records, "a scheduled launch that never started must not leave a row the sidebar lists")

	launchBlocked("by hand", "blocked2", false)
	records, err = svc.sessions.List(t.Context(), "demo")
	require.NoError(t, err)
	assert.Len(t, records, 1, "a launch the user made keeps its record to retry from")
}

// TestLaunchAcceptsAnAgentNoConfigNames is the regression this schema change
// exists for. Both cases used to be hard refusals at launch: an agent absent
// from hive's config, and one present there but absent from agentws's launch
// table ("pi"). Neither is a gate now — the command template is.
func TestLaunchAcceptsAnAgentNoConfigNames(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()

	for _, tc := range []struct{ dir, agent string }{
		{"unknown-to-hive", "mystery-agent"},
		{"unknown-to-agentws", "pi"},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			agentCmd := fakeAgentBinary(t, tc.agent, "cat")
			writeAgentWorkspaceManifest(t, root, tc.dir, agentWorkspaceYAML("Demo", agentCmd, ""))
			svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

			started, err := svc.StartSession(t.Context(), StartSession{Workspace: tc.dir, Name: "s1", Cols: 80, Rows: 24})
			require.NoError(t, err)
			assert.NotEmpty(t, started.TerminalID, "the session is live")
			require.NoError(t, svc.DeleteSession(t.Context(), started.ID))
		})
	}
}

// TestLaunchRefusesABrokenCommandTemplate is what replaced the unknown-agent
// refusals: the only launch-time refusal left is the workspace's own template,
// and it names the workspace rather than an agent key.
func TestLaunchRefusesABrokenCommandTemplate(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "broken",
		agentWorkspaceYAML("Broken", "claude {{ .Nope }}", ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	// A template this broken fails to load at all, so the workspace is listed
	// with a problem rather than started -- the earlier of the two refusals.
	_, err := svc.StartSession(t.Context(), StartSession{Workspace: "broken", Name: "s1", Cols: 80, Rows: 24})
	require.Error(t, err)
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestTwoSessionsMayShareAName(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agentCmd := fakeAgentBinary(t, "claude", "cat")
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agentCmd, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})

	first, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "dup", Cols: 80, Rows: 24})
	require.NoError(t, err)
	second, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "dup", Cols: 80, Rows: 24})
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, second.ID, "the row id is the identity, not the name")
	assert.NotEqual(t, first.TerminalID, second.TerminalID)

	result, err := svc.Open(t.Context(), "demo")
	require.NoError(t, err)
	assert.Len(t, result.Sessions, 2)
}

func TestSessionsListsWithoutRegeneratingArtifacts(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agentCmd := fakeAgentBinary(t, "claude", "cat")
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agentCmd, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})

	started, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "s1", Cols: 80, Rows: 24})
	require.NoError(t, err)

	sessions, err := svc.Sessions(t.Context(), "demo")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, started.ID, sessions[0].ID)
	assert.Equal(t, started.TerminalID, sessions[0].TerminalID)

	_, err = os.Stat(filepath.Join(root, "demo", ".mcp.json"))
	assert.True(t, os.IsNotExist(err), "Sessions must not regenerate the workspace's disposable artifacts")

	_, err = svc.Sessions(t.Context(), "../escape")
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestDeleteWorkspaceRemovesTheDirectoryAndTheRecords(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "demo", "version: 2\nname: Demo\nagent: claude\ncommand: true\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, "demo", "AGENTS.md"), []byte("# Demo\n"), 0o600))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	_, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "s1", Cols: 80, Rows: 24})
	require.NoError(t, err)

	// Schedule state is app-local like the session records, and a cursor left
	// behind would back-fire for a workspace rebuilt under the same name.
	require.NoError(t, svc.schedules.SaveCursor(t.Context(), stores.ScheduleCursor{
		Workspace: "demo", ScheduleID: "weekly", EvaluatedThrough: 100, Cron: "@weekly",
	}))
	_, err = svc.schedules.InsertRun(t.Context(), stores.ScheduleRun{
		Workspace: "demo", ScheduleID: "weekly", ScheduleName: "weekly",
		ScheduledFor: 100, StartedAt: 100, Reason: "due", Status: "launched", SessionID: 1,
	})
	require.NoError(t, err)

	require.NoError(t, svc.DeleteWorkspace(t.Context(), "demo"))

	sessions, err := svc.sessions.List(t.Context(), "demo")
	require.NoError(t, err)
	assert.Empty(t, sessions)

	runs, err := svc.schedules.ListRuns(t.Context(), "demo", "weekly", 10)
	require.NoError(t, err)
	assert.Empty(t, runs)
	_, ok, err := svc.schedules.Cursor(t.Context(), "demo", "weekly")
	require.NoError(t, err)
	assert.False(t, ok)

	assert.NoDirExists(t, filepath.Join(root, "demo"))
	listed, err := svc.List(t.Context())
	require.NoError(t, err)
	assert.Empty(t, listed, "the row is gone from the next scan, not just from the caller's copy")
	assert.DirExists(t, root, "only the workspace goes, never the root around it")
}

func TestDeleteWorkspaceRefusesAPathTheRootDoesNotList(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "demo", "version: 2\nname: Demo\nagent: claude\ncommand: true\n")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".shared", "skills"), 0o700))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	for _, tc := range []struct {
		dir  string
		kind Kind
	}{
		{"../escape", KindInvalid},
		{"", KindInvalid},
		{".shared", KindNotFound},
		{"never-created", KindNotFound},
	} {
		err := svc.DeleteWorkspace(t.Context(), tc.dir)
		require.Error(t, err, "dir %q", tc.dir)
		assert.Equal(t, tc.kind, KindOf(err), "dir %q", tc.dir)
	}

	assert.DirExists(t, filepath.Join(root, ".shared", "skills"))
	assert.DirExists(t, filepath.Join(root, "demo"))
}

func TestAllSessionsSpansEveryWorkspaceInStableCreationOrder(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agentCmd := fakeAgentBinary(t, "claude", "cat")
	writeAgentWorkspaceManifest(t, root, "demo-a", agentWorkspaceYAML("Demo A", agentCmd, ""))
	writeAgentWorkspaceManifest(t, root, "demo-b", "version: 2\nname: Demo B\nagent: claude\ncommand: true\n")
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})

	a1, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo-a", Name: "a1", Cols: 80, Rows: 24})
	require.NoError(t, err)
	b1, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo-b", Name: "b1", Cols: 80, Rows: 24})
	require.NoError(t, err)

	// Resuming a1 touches last_opened_at but must not move its row: the list
	// keeps creation order so the sidebar never reshuffles under the pointer.
	_, err = svc.ResumeSession(t.Context(), a1.ID, 80, 24)
	require.NoError(t, err)

	all, err := svc.AllSessions(t.Context())
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, b1.ID, all[0].ID)
	assert.Equal(t, "demo-b", all[0].Workspace)
	assert.Equal(t, a1.ID, all[1].ID)
	assert.Equal(t, "demo-a", all[1].Workspace)
}

func TestRenameSession(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agentCmd := fakeAgentBinary(t, "claude", "cat")
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agentCmd, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})

	started, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "New Chat", Cols: 80, Rows: 24})
	require.NoError(t, err)

	require.NoError(t, svc.RenameSession(t.Context(), started.ID, "  fix the flaky test  "))
	sessions, err := svc.Sessions(t.Context(), "demo")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "fix the flaky test", sessions[0].Name, "the stored name is trimmed")
	assert.Equal(t, started.TerminalID, sessions[0].TerminalID, "the live terminal is untouched by a rename")

	err = svc.RenameSession(t.Context(), started.ID, "   ")
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))

	err = svc.RenameSession(t.Context(), 999999, "ghost")
	require.Error(t, err)
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestCreateAndUpdateWorkspace(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	svc := newManifestOnlyService(t, root, map[string]string{"claude": "true", "codex": "true"})

	created, err := svc.CreateWorkspace(t.Context(), WorkspaceEdit{Dir: "demo", Name: "Demo", Command: "claude"})
	require.NoError(t, err)
	assert.Equal(t, "demo", created.Dir)
	assert.Equal(t, "Demo", created.Name)
	assert.Equal(t, "claude", created.Command)
	assert.FileExists(t, filepath.Join(root, "demo", "agent-workspace.yaml"))

	_, err = svc.CreateWorkspace(t.Context(), WorkspaceEdit{Dir: "demo", Name: "Demo", Command: "claude"})
	require.ErrorContains(t, err, "already exists")

	updated, err := svc.UpdateWorkspace(t.Context(), WorkspaceEdit{Dir: "demo", Name: "Renamed", Command: "codex --sandbox workspace-write"})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", updated.Name)
	assert.Equal(t, "codex --sandbox workspace-write", updated.Command)

	list, err := svc.List(t.Context())
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Renamed", list[0].Name)

	_, err = svc.UpdateWorkspace(t.Context(), WorkspaceEdit{Dir: "demo", Name: "X", Command: "no-such-agent"})
	require.NoError(t, err, "a CLI this build does not know is not a refusal")

	_, err = svc.UpdateWorkspace(t.Context(), WorkspaceEdit{Dir: "demo", Name: "X", Command: "claude {{ .Nope }}"})
	require.ErrorContains(t, err, "command template", "a template the launch would refuse is refused by the editor that wrote it")
	assert.Equal(t, KindInvalid, KindOf(err))
	_, err = svc.UpdateWorkspace(t.Context(), WorkspaceEdit{Dir: "missing", Name: "X", Command: "claude"})
	require.ErrorContains(t, err, "not found")
}

func TestWorkspaceEditOwnsTheMCPList(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	svc := newManifestOnlyService(t, root, map[string]string{"claude": "true"})

	created, err := svc.CreateWorkspace(t.Context(), WorkspaceEdit{Dir: "demo", Name: "Demo", Command: "claude", MCPs: []string{"playwright"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"playwright"}, created.MCPs)
	assert.FileExists(t, filepath.Join(root, "demo", "AGENTS.md"), "a created workspace starts with prose to shape")

	updated, err := svc.UpdateWorkspace(t.Context(), WorkspaceEdit{Dir: "demo", Name: "Demo", Command: "claude"})
	require.NoError(t, err)
	assert.Empty(t, updated.MCPs)

	_, err = svc.UpdateWorkspace(t.Context(), WorkspaceEdit{Dir: "demo", Name: "Demo", Command: "claude", MCPs: []string{"playwright", "playwright"}})
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestWorkspaceEditOwnsTheSkillPackageList(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	svc := newManifestOnlyService(t, root, map[string]string{"claude": "true"})

	created, err := svc.CreateWorkspace(t.Context(), WorkspaceEdit{
		Dir: "demo", Name: "Demo", Command: "claude", Skills: []string{"hive"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"hive"}, created.Skills)

	updated, err := svc.UpdateWorkspace(t.Context(), WorkspaceEdit{Dir: "demo", Name: "Demo", Command: "claude"})
	require.NoError(t, err)
	assert.Empty(t, updated.Skills)

	_, err = svc.UpdateWorkspace(t.Context(), WorkspaceEdit{
		Dir: "demo", Name: "Demo", Command: "claude", Skills: []string{"hive", "hive"},
	})
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
}

// Schedules are a manifest key like mcps: and skills:, so they are saved with
// the rest of the manifest and reconciled to exactly what the editor sends.
func TestWorkspaceEditOwnsTheScheduleList(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	var changed []string
	svc.OnSchedulesChanged = func(workspace string) { changed = append(changed, workspace) }

	base := WorkspaceEdit{Dir: "demo", Name: "Demo", Command: "claude" + agentws.PromptTail}
	created := base
	created.Schedules = []ScheduleEdit{
		{ID: "weekly", Name: "Weekly summary", Cron: "0 9 * * 5", Prompt: "Summarize the week."},
		{ID: "daily", Cron: "@daily", Prompt: "Standup."},
	}
	view, err := svc.CreateWorkspace(t.Context(), created)
	require.NoError(t, err)
	require.Len(t, view.Schedules, 2)
	assert.Equal(t, "weekly", view.Schedules[0].ID)
	require.NotNil(t, view.Schedules[0].NextRunAt)
	assert.Nil(t, view.Schedules[0].LastRun)
	assert.Equal(t, []string{"demo"}, changed, "a write has to reach the running scheduler")

	updated := base
	updated.Schedules = []ScheduleEdit{
		{ID: "weekly", Name: "Weekly summary", Cron: "0 10 * * 1", Prompt: "Summarize the week.", Disabled: true},
	}
	view, err = svc.UpdateWorkspace(t.Context(), updated)
	require.NoError(t, err)
	require.Len(t, view.Schedules, 1, "an entry the editor stopped naming is deleted by the same write")
	assert.Equal(t, "0 10 * * 1", view.Schedules[0].Cron)
	assert.True(t, view.Schedules[0].Disabled)

	view, err = svc.UpdateWorkspace(t.Context(), base)
	require.NoError(t, err)
	assert.Empty(t, view.Schedules)
	assert.Len(t, changed, 3)
}

func TestWorkspaceEditRejectsAnInvalidSchedule(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	base := WorkspaceEdit{Dir: "demo", Name: "Demo", Command: "claude" + agentws.PromptTail}
	_, err := svc.CreateWorkspace(t.Context(), base)
	require.NoError(t, err)

	var changed []string
	svc.OnSchedulesChanged = func(workspace string) { changed = append(changed, workspace) }

	badCron := base
	badCron.Name = "Renamed"
	badCron.Schedules = []ScheduleEdit{{ID: "weekly", Cron: "not a cron", Prompt: "hi"}}
	_, err = svc.UpdateWorkspace(t.Context(), badCron)
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Contains(t, err.Error(), "not a cron", "the reason has to reach the editor")

	duplicate := base
	duplicate.Schedules = []ScheduleEdit{
		{ID: "weekly", Cron: "@daily", Prompt: "hi"},
		{ID: "weekly", Cron: "@weekly", Prompt: "hi"},
	}
	_, err = svc.UpdateWorkspace(t.Context(), duplicate)
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))

	raw, err := os.ReadFile(filepath.Join(root, "demo", "agent-workspace.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "schedules", "a rejected edit writes nothing")
	assert.NotContains(t, string(raw), "Renamed", "not even the fields the edit got right")
	assert.Empty(t, changed, "nothing was written, so the scheduler has nothing to re-read")
}

// A manifest the loader could not read is not something the editor may write
// over: its form loads from the workspace view, and on the first load of a run
// there is no last-good snapshot behind the break, so a save would reconcile
// every list in the file to nothing.
func TestUpdateWorkspaceRefusesABrokenManifest(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "demo", brokenScheduleManifest)
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	path := filepath.Join(root, "demo", "agent-workspace.yaml")
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	_, err = svc.UpdateWorkspace(t.Context(), WorkspaceEdit{
		Dir: "demo", Name: "Renamed", Command: "claude",
	})
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Contains(t, err.Error(), "agent-workspace.yaml has a problem")
	assert.Contains(t, err.Error(), "not a cron", "the reason names what to fix")

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "a refused edit leaves the file byte for byte")
}

// A chat ends itself by presenting the token its launch handed it. It is
// deleted after the configured grace rather than inside the call, so the tool
// call that made the request returns first, and the record goes with the tmux
// session: a schedule must not leave a row per run in the sidebar.
func TestEndOwnSessionDeletesTheChatAfterTheDelay(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agentCmd := fakeAgentBinary(t, "claude", "cat")
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agentCmd, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})
	svc.endDelay = FixedSessionEndDelay(100 * time.Millisecond)
	ended := make(chan events.SchedulesUpdated, 1)
	cancel := events.Subscribe(t.Context(), svc.events, "test.ended", events.Buffer(1), func(_ context.Context, e events.SchedulesUpdated) { ended <- e })
	t.Cleanup(cancel)

	started, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "s1", Cols: 80, Rows: 24, ScheduleID: "weekly"})
	require.NoError(t, err)
	rec, err := svc.sessions.Get(t.Context(), started.ID)
	require.NoError(t, err)
	require.NotEmpty(t, rec.EndToken, "every launch mints a token")

	_, err = svc.EndOwnSession(t.Context(), "not-a-token")
	require.Error(t, err)
	assert.Equal(t, KindUnauthenticated, KindOf(err))

	ending, err := svc.EndOwnSession(t.Context(), rec.EndToken)
	require.NoError(t, err)
	assert.Equal(t, started.ID, ending.Session.ID)
	assert.WithinDuration(t, time.Now().Add(100*time.Millisecond), ending.EndsAt, time.Second)

	live, err := svc.SessionLive(t.Context(), started.ID)
	require.NoError(t, err)
	assert.True(t, live, "the session outlives the call itself")

	select {
	case e := <-ended:
		assert.Equal(t, "demo", e.Workspace, "a scheduled chat that ended itself wakes the Chats area")
	case <-time.After(5 * time.Second):
		t.Fatal("the session was not ended after the delay")
	}
	live, err = svc.SessionLive(t.Context(), started.ID)
	require.NoError(t, err)
	assert.False(t, live)

	sessions, err := svc.Sessions(t.Context(), "demo")
	require.NoError(t, err)
	assert.Empty(t, sessions, "the record goes with the tmux session")
}

// A scheduled launch hands the agent the framed prompt and the means to end
// itself: the frame names the schedule and carries the curl, and the process
// environment carries the token and the URL the curl reads.
func TestStartScheduledSessionFramesThePromptAndHandsOutTheToken(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	captured := filepath.Join(t.TempDir(), "launch")
	agentCmd := fakeAgentBinary(t, "claude", `sh -c 'printf "%s\n" "$@" > "$0.args"; env > "$0.env"; exec cat' `+captured+` "$@"`)
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agentCmd+agentws.PromptTail, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})
	svc.mcpBase = mcpBaseFunc(func(context.Context) string { return "http://127.0.0.1:4321" })

	started, err := svc.StartScheduledSession(t.Context(), StartScheduledSession{
		Workspace: "demo", ScheduleID: "weekly", ScheduleName: "Weekly summary",
		Name: "Weekly summary - Sep 6 09:00", Prompt: "Summarize the week.",
	})
	require.NoError(t, err)
	rec, err := svc.sessions.Get(t.Context(), started.ID)
	require.NoError(t, err)
	assert.Equal(t, "weekly", rec.ScheduleID, "the record wears its schedule's id, which is what marks the row in the sidebar")

	var args, env string
	require.Eventually(t, func() bool {
		a, errA := os.ReadFile(captured + ".args")
		e, errE := os.ReadFile(captured + ".env")
		args, env = string(a), string(e)
		return errA == nil && errE == nil
	}, 5*time.Second, 25*time.Millisecond, "the fake agent records what it was launched with")

	assert.Contains(t, args, `scheduled task "Weekly summary" in the "Demo" workspace`)
	assert.Contains(t, args, "Summarize the week.")
	assert.Contains(t, args, "you MUST end this session")
	assert.Contains(t, env, "HIVE_AGENT_SESSION_TOKEN="+rec.EndToken+"\n")
	assert.Contains(t, env, "HIVE_AGENT_SESSION_END_URL=http://127.0.0.1:4321"+AgentSessionEndPath+"\n")
}

// PutSchedule and RemoveSchedule are the MCP tools' writes: one entry at a
// time, in place, with the rest of the manifest and its comments untouched,
// and the same refusal of a broken manifest the editor's write has.
func TestPutAndRemoveScheduleEditOneEntryInPlace(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "demo", "# keep me\n"+agentWorkspaceYAML("Demo", "claude"+agentws.PromptTail, "mcps: [playwright]\n"))
	writeAgentWorkspaceManifest(t, root, "broken", brokenScheduleManifest)
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})
	changed := 0
	svc.OnSchedulesChanged = func(string) { changed++ }

	row, err := svc.PutSchedule(t.Context(), "demo", SchedulePatch{
		ID: "weekly", Name: new("Weekly summary"), Cron: new("0 9 * * 5"), Prompt: new("Summarize the week."),
	})
	require.NoError(t, err)
	assert.Equal(t, "weekly", row.ID)
	require.NotNil(t, row.NextRunAt)
	assert.Equal(t, 1, changed, "a write reaches the scheduler and the UI")

	_, err = svc.PutSchedule(t.Context(), "demo", SchedulePatch{ID: "daily", Cron: new("@daily"), Prompt: new("Standup.")})
	require.NoError(t, err)
	row, err = svc.PutSchedule(t.Context(), "demo", SchedulePatch{ID: "weekly", Disabled: new(true), OnMissed: new("skip")})
	require.NoError(t, err)
	assert.True(t, row.Disabled)
	assert.Equal(t, "Weekly summary", row.Name, "a field the patch omits keeps its stored value")
	assert.Equal(t, "0 9 * * 5", row.Cron)

	// Re-timing a paused schedule leaves it paused: the case an agent hits
	// when told "move that schedule to 10:00" about one the user switched off.
	row, err = svc.PutSchedule(t.Context(), "demo", SchedulePatch{ID: "weekly", Cron: new("0 10 * * 5")})
	require.NoError(t, err)
	assert.Equal(t, "0 10 * * 5", row.Cron)
	assert.True(t, row.Disabled)
	assert.Equal(t, "skip", row.OnMissed)
	assert.Equal(t, "Weekly summary", row.Name)
	row, err = svc.PutSchedule(t.Context(), "demo", SchedulePatch{ID: "weekly", Name: new("")})
	require.NoError(t, err)
	assert.Empty(t, row.Name, "an explicit empty name clears the stored one")

	raw, err := os.ReadFile(filepath.Join(root, "demo", "agent-workspace.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "# keep me")
	assert.Contains(t, string(raw), "playwright")
	st, ok := svc.store.Status("demo")
	require.True(t, ok)
	require.Len(t, st.Workspace.Schedules, 2, "an existing id is replaced, not appended")
	assert.Equal(t, "weekly", st.Workspace.Schedules[0].ID)

	_, err = svc.PutSchedule(t.Context(), "demo", SchedulePatch{ID: "bad", Cron: new("not a cron"), Prompt: new("go")})
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
	_, err = svc.PutSchedule(t.Context(), "demo", SchedulePatch{ID: "new", Cron: new("@daily")})
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err), "a new schedule needs a cron and a prompt")
	require.ErrorContains(t, err, "does not exist")
	_, err = svc.PutSchedule(t.Context(), "broken", SchedulePatch{ID: "weekly", Cron: new("@daily"), Prompt: new("go")})
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err), "a broken manifest is fixed in the file, never written over")
	_, err = svc.PutSchedule(t.Context(), "nope", SchedulePatch{ID: "weekly", Cron: new("@daily"), Prompt: new("go")})
	require.Error(t, err)
	assert.Equal(t, KindNotFound, KindOf(err))

	require.NoError(t, svc.RemoveSchedule(t.Context(), "demo", "weekly"))
	st, _ = svc.store.Status("demo")
	require.Len(t, st.Workspace.Schedules, 1)
	assert.Equal(t, "daily", st.Workspace.Schedules[0].ID)
	err = svc.RemoveSchedule(t.Context(), "demo", "weekly")
	require.Error(t, err)
	assert.Equal(t, KindNotFound, KindOf(err))
	assert.Equal(t, 6, changed)
}

// TestSkillPackagesResolveMembers is the editor's read: packages from
// skills.yml, each carrying the skills its patterns currently select across
// both sources.
func TestSkillPackagesResolveMembers(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeSharedSkill(t, root, "terraform-plan", "# terraform plan\n")
	writeSharedSkill(t, root, "runbook", "# runbook\n")
	writeSkillPackages(t, root, "version: 1\npackages:\n"+
		"  hive:\n    title: Hive\n    include: [\"hive-*\"]\n    exclude: [\"hive-settings\"]\n"+
		"  infra:\n    include: [\"terraform-*\", \"runbook\"]\n")

	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	items, names, problem := svc.SkillPackages(t.Context())
	assert.Empty(t, problem)
	byName := map[string]SkillPackageItem{}
	for _, item := range items {
		byName[item.Name] = item
	}

	infra, ok := byName["infra"]
	require.True(t, ok)
	slugs := make([]string, 0, len(infra.Members))
	for _, m := range infra.Members {
		slugs = append(slugs, m.Slug)
		assert.False(t, m.Shipped, "a shared skill is not shipped")
	}
	assert.Equal(t, []string{"runbook", "terraform-plan"}, slugs)

	hive, ok := byName["hive"]
	require.True(t, ok)
	assert.Equal(t, "Hive", hive.Title)
	require.NotEmpty(t, hive.Members, "the shipped skills are in the name-space packages match")
	for _, m := range hive.Members {
		assert.True(t, m.Shipped)
		assert.NotEqual(t, "hive-settings", m.Slug, "exclude carves a member out")
	}

	// The name-space rides along so the editor can say what an enabled name
	// that is not a package actually is.
	bySlug := map[string]SkillNameItem{}
	for _, name := range names {
		bySlug[name.Slug] = name
	}
	assert.Equal(t, []string{"infra"}, bySlug["runbook"].SelectedBy)
	assert.Equal(t, []string{"hive"}, bySlug["hive-flows"].SelectedBy)
	assert.Empty(t, bySlug["hive-settings"].SelectedBy, "an excluded skill is selected by nothing")
}

// TestOpenInstallsWhatThePackagesSelect is the scoping packages exist for: a
// workspace carries a skill because a package it enabled selects it, and a
// package skills.yml does not define is reported rather than failing the open.
func TestOpenInstallsWhatThePackagesSelect(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeSharedSkill(t, root, "terraform-plan", "# Terraform plan\n\nRun it.\n")
	writeSharedSkill(t, root, "release-notes", "# Release notes\n")
	writeSkillPackages(t, root, "version: 1\npackages:\n  infra:\n    include: [\"terraform-*\"]\n")
	writeAgentWorkspaceManifest(t, root, "with", "version: 2\nname: With\nagent: claude\ncommand: true\n"+
		"skills:\n  - infra\n  - ghost\n")
	writeAgentWorkspaceManifest(t, root, "without", "version: 2\nname: Without\nagent: claude\ncommand: true\n")

	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	result, err := svc.Open(t.Context(), "with")
	require.NoError(t, err, "a package with no definition does not fail the open")
	assert.Equal(t, []MissingPackageItem{{Name: "ghost"}}, result.MissingPackages,
		"a name matching no skill either is reported as the typo it is")

	body, err := os.ReadFile(filepath.Join(root, "with", ".claude", "skills", "terraform-plan", "SKILL.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Terraform plan\n\nRun it.\n", string(body), "a shared skill installs verbatim")
	assert.NoDirExists(t, filepath.Join(root, "with", ".claude", "skills", "release-notes"),
		"a shared skill no enabled package selects is not carried")

	_, err = svc.Open(t.Context(), "without")
	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(root, "without", ".claude", "skills", "terraform-plan"),
		"packages are shared; enabling one is the workspace's own choice")
}

// TestOpenAddsASkillToEveryWorkspaceThatMatched is the payoff over naming
// skills one by one: a new file matching a package's pattern reaches every
// workspace that enabled it, with no manifest edited.
func TestOpenAddsASkillToEveryWorkspaceThatMatched(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeSharedSkill(t, root, "terraform-plan", "# plan\n")
	writeSkillPackages(t, root, "version: 1\npackages:\n  infra:\n    include: [\"terraform-*\"]\n")
	writeAgentWorkspaceManifest(t, root, "demo", "version: 2\nname: Demo\nagent: claude\ncommand: true\nskills:\n  - infra\n")

	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})
	_, err := svc.Open(t.Context(), "demo")
	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(root, "demo", ".claude", "skills", "terraform-apply"))

	writeSharedSkill(t, root, "terraform-apply", "# apply\n")
	_, err = svc.Open(t.Context(), "demo")
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(root, "demo", ".claude", "skills", "terraform-apply", "SKILL.md"),
		"the pattern picked the new skill up with no manifest change")
}

func TestImportAndRemoveMCPServers(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	added, err := svc.ImportMCPServers(t.Context(), `{"mcpServers": {"paperless": {"command": "uvx", "args": ["paperless-mcp"]}}}`)
	require.NoError(t, err)
	assert.Equal(t, []string{"paperless"}, added)

	catalogue := svc.MCPCatalogue(t.Context())
	byID := make(map[string]MCPCatalogueItem, len(catalogue))
	for _, item := range catalogue {
		byID[item.ID] = item
	}
	require.Contains(t, byID, "paperless")
	assert.Equal(t, "uvx paperless-mcp", byID["paperless"].Command)
	assert.False(t, byID["paperless"].Shipped)
	require.Contains(t, byID, "playwright", "the shipped catalogue is part of the merged view")
	assert.True(t, byID["playwright"].Shipped)

	_, err = svc.ImportMCPServers(t.Context(), `{"paperless": {"command": "uvx"}}`)
	require.Error(t, err)
	assert.Equal(t, KindConflict, KindOf(err))

	require.NoError(t, svc.RemoveMCPServer(t.Context(), "paperless"))
	err = svc.RemoveMCPServer(t.Context(), "paperless")
	require.Error(t, err)
	assert.Equal(t, KindNotFound, KindOf(err))

	err = svc.RemoveMCPServer(t.Context(), "playwright")
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err), "a shipped entry is disabled per workspace, never removed")
}

// The desktop's own entries ship with no URL — the loopback port is allocated
// at startup — so the catalogue is what substitutes this run's live base
// joined with each entry's RuntimePath. A workspace generated against a
// static placeholder would point its agent at an address nothing answers.
func TestCatalogueResolvesTheDesktopsOwnEndpoint(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	byID := make(map[string]MCPCatalogueItem)
	for _, item := range svc.MCPCatalogue(t.Context()) {
		byID[item.ID] = item
	}

	require.Contains(t, byID, "hive-desktop")
	assert.True(t, byID["hive-desktop"].Shipped)
	assert.Equal(t, testMCPBaseURL+"/mcp", byID["hive-desktop"].Command)
	assert.Empty(t, byID["hive-desktop"].Problem)

	require.Contains(t, byID, "hive-canvas")
	assert.True(t, byID["hive-canvas"].Shipped)
	assert.Equal(t, testMCPBaseURL+"/mcp/canvas", byID["hive-canvas"].Command)
	assert.Empty(t, byID["hive-canvas"].Problem)
}

// With the loopback server down there is no endpoint to resolve, and the entry
// has to say so rather than render an empty URL that looks configured.
func TestCatalogueReportsAProblemWhenTheServerIsDown(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})
	svc.mcpBase = mcpBaseFunc(func(context.Context) string { return "" })

	byID := make(map[string]MCPCatalogueItem)
	for _, item := range svc.MCPCatalogue(t.Context()) {
		byID[item.ID] = item
	}

	require.Contains(t, byID, "hive-desktop")
	assert.Empty(t, byID["hive-desktop"].Command)
	assert.Contains(t, byID["hive-desktop"].Problem, "http.enabled")
}

func mcpCatalogueItem(t *testing.T, svc *AgentWorkspacesService, id string) MCPCatalogueItem {
	t.Helper()
	for _, item := range svc.MCPCatalogue(t.Context()) {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("no catalogue entry %q", id)
	return MCPCatalogueItem{}
}

// A stdio command is validated against the PATH a session launches with, not
// the one this process inherited: a desktop launch gets launchd's
// /usr/bin:/bin:/usr/sbin:/sbin, so validating against it warned on every
// stdio entry — the shipped npx ones included — on a stock install (hay-kot/hive-desktop#266).
func TestMCPCatalogueValidatesAgainstTheResolvedPATH(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	binDir := t.TempDir()
	const command = "hive-test-mcp-command"
	require.NoError(t, os.WriteFile(filepath.Join(binDir, command), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	writeMCPLibrary(t, root, "version: 1\nservers:\n  local:\n    command: "+command+"\n")

	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})
	require.NotEmpty(t, mcpCatalogueItem(t, svc, "local").Problem,
		"the command is on no PATH this process inherited")

	svc.execEnv = execenv.NewResolver(execenv.Options{Shell: "/bin/sh", Probe: func(context.Context, string) (map[string]string, error) {
		return map[string]string{"PATH": binDir}, nil
	}})
	assert.Empty(t, mcpCatalogueItem(t, svc, "local").Problem,
		"the resolver finds it on the login shell's PATH, so the entry must not warn")
}

// A workspace declaring the desktop gets the live endpoint written into its
// generated .mcp.json, which is the whole point of the entry.
func TestGeneratedMCPConfigCarriesTheLiveEndpoint(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "demo",
		"version: 2\nname: Demo\nagent: claude\ncommand: true\nmcps:\n  - hive-desktop\n")
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	res, err := svc.Open(t.Context(), "demo")
	require.NoError(t, err)
	assert.Empty(t, res.MissingMCPs)

	generated, err := os.ReadFile(filepath.Join(root, "demo", ".mcp.json"))
	require.NoError(t, err)
	assert.Contains(t, string(generated), testMCPBaseURL+"/mcp")
}

func TestOpenWorkspaceInEditor(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeAgentWorkspaceManifest(t, root, "demo", "version: 2\nname: Demo\nagent: claude\ncommand: true\n")
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	err := svc.OpenWorkspaceInEditor(t.Context(), "demo")
	require.Error(t, err, "no editor configured")
	assert.Equal(t, KindInvalid, KindOf(err))

	err = svc.RevealWorkspace(t.Context(), "ghost")
	require.Error(t, err)
	assert.Equal(t, KindNotFound, KindOf(err))

	fake := fakeAgentBinary(t, "editor", "true")
	svc.execEnv = execenv.NewResolver(execenv.Options{Shell: "/bin/sh", Probe: func(context.Context, string) (map[string]string, error) {
		return map[string]string{"PATH": "/usr/bin:/bin"}, nil
	}})
	svc.editorCommand = editorCommandFunc(func(context.Context) (string, error) { return fake, nil })
	command, title := svc.Editor(t.Context())
	assert.Equal(t, fake, command)
	assert.Equal(t, fake, title, "a command outside the known catalogue labels itself")
	require.NoError(t, svc.OpenWorkspaceInEditor(t.Context(), "demo"))
}

func TestResizeSessionResizesTheLiveTerminal(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	agentCmd := fakeAgentBinary(t, "claude", "cat")
	writeAgentWorkspaceManifest(t, root, "demo", agentWorkspaceYAML("Demo", agentCmd, ""))
	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": agentCmd})

	started, err := svc.StartSession(t.Context(), StartSession{Workspace: "demo", Name: "s1", Cols: 80, Rows: 24})
	require.NoError(t, err)
	require.NotEmpty(t, started.TerminalID)

	require.NoError(t, svc.ResizeSession(t.Context(), started.ID, 120, 30))

	client, ok := svc.terminals.Client(started.TerminalID)
	require.True(t, ok)
	// The %layout-change answering the vote is asynchronous; the window
	// settles shortly after. Width is the assertable half — tmux may keep a
	// row for its status line, so height is its call.
	require.Eventually(t, func() bool {
		windows := client.Windows()
		return len(windows) > 0 && windows[0].Width == 120
	}, 3*time.Second, 25*time.Millisecond, "the size vote must reach the tmux window")

	err = svc.ResizeSession(t.Context(), started.ID+999, 80, 24)
	require.Error(t, err)
}

// TestOpenNamesTheFixWhenAnEnabledNameIsASkill is hay-kot/hive-desktop#307: a manifest written
// before packages were the enablement unit enumerates skill slugs, and the
// bare "not defined in skills.yml" that produces is accurate and useless.
// The report has to say the name is a skill and which package already
// carries it, because that toggle is the whole fix.
func TestOpenNamesTheFixWhenAnEnabledNameIsASkill(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	writeSkillPackages(t, root, "version: 1\npackages:\n  hive:\n    include: [\"hive-*\"]\n")
	writeAgentWorkspaceManifest(t, root, "legacy", "version: 3\nname: Legacy\nagent: claude\ncommand: true\n"+
		"skills:\n  - hive-flows\n  - nonsense\n")

	svc := newTestAgentWorkspacesService(t, root, map[string]string{"claude": "true"})

	result, err := svc.Open(t.Context(), "legacy")
	require.NoError(t, err)
	assert.Equal(t, []MissingPackageItem{
		{Name: "hive-flows", Skill: true, SelectedBy: []string{"hive"}},
		{Name: "nonsense"},
	}, result.MissingPackages)
}

// TestPresetsSeedFromHiveProfiles covers the seam this change reversed: hive's
// profiles used to reach a workspace with their flags stripped, and now reach
// the editor as presets carrying them. A shipped agent is not duplicated, and
// a profile running a CLI this build knows gets that CLI's wiring appended so
// the preset is launchable rather than merely present.
func TestPresetsSeedFromHiveProfiles(t *testing.T) {
	isolateConfig(t)
	root := t.TempDir()
	svc := newManifestOnlyService(t, root, map[string]string{
		"claude": "claude --dangerously-skip-permissions",
		"pi":     "pi",
		"fable":  "claude --dangerously-skip-permissions --model claude-fable-5",
	})

	byID := map[string]agentws.Preset{}
	for _, p := range svc.Presets(t.Context()) {
		byID[p.ID] = p
	}

	require.Contains(t, byID, "claude-ask", "the shipped postures are still offered")
	assert.NotContains(t, byID, "hive-claude",
		"a shipped agent's own presets are more precise than hive's single profile line")

	pi, ok := byID["hive-pi"]
	require.True(t, ok, "an agent only hive names is offered")
	assert.Equal(t, "pi", pi.Command)
	assert.False(t, pi.Danger)

	fable, ok := byID["hive-fable"]
	require.True(t, ok)
	assert.Equal(t, "fable", fable.Label, "the row keeps the profile key the user recognizes")
	assert.Equal(t, "claude", fable.Agent, "the mark beside it is the CLI the command actually runs")
	assert.Contains(t, fable.Command, "--model claude-fable-5", "the profile's flags cross, unlike before")
	assert.Contains(t, fable.Command, "--strict-mcp-config", "a profile running claude gets claude's wiring")
	assert.True(t, fable.Danger)

	// Every preset the editor offers must produce a manifest that launches.
	for id, p := range byID {
		require.NoError(t, agentws.ValidateCommand(p.Command), "preset %q", id)
	}
}
