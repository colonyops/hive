package mcpsrv

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/hive/prompt"
)

func (ctrl *OrchestratorController) register(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:  "list_repositories",
		Title: "List repositories",
		Description: "List the repositories hive can start a session in (name and remote), the agent profiles it can run, " +
			"and the defaults. Pass a repository's remote or name to start_session.",
	}, ctrl.ListRepositories)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "start_session",
		Title: "Start a hive session",
		Description: "Create a hive session: an isolated checkout of a repository with an agent launched in tmux and " +
			"given prompt as its first message. Every session started here is tagged orchestrator, so list_sessions " +
			"finds your fleet. Answers once the checkout exists and the agent is spawning; a first clone can take a " +
			"while. A name already in use is a conflict. One session per repository per piece of work; write the prompt " +
			"as a contract: the workflow, the deliverable, and when to stop and report.",
	}, ctrl.StartSession)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "list_sessions",
		Title: "List hive sessions",
		Description: "List hive sessions, newest first, with each agent's published status (active, approval, ready, " +
			"missing). By default only active sessions tagged orchestrator; all=true lists every session in every " +
			"state, and tags narrows to sessions carrying every tag given.",
	}, ctrl.ListSessions)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "peek_session",
		Title: "Read a session's agent",
		Description: "Capture a session's agent pane: the screen tail, context window use when the status bar shows it, " +
			"and a state hint: active, approval, question (including a folder-trust dialog), ready, unknown, or missing " +
			"(no agent pane). The hint can be wrong; the tail is what the agent shows. list_sessions and " +
			"wait_for_session report question as approval.",
	}, ctrl.PeekSession)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "send_prompt",
		Title: "Type a prompt into a session",
		Description: "Type text into a session's agent, wait for it to render, press Enter once, and return the screen " +
			"as peek_session does. Text is typed as-is, whatever the agent shows, and appended to anything already in its " +
			"input box. If the text is still in the input box afterwards, the Enter was not taken: press Enter with " +
			"send_keys. Claude Code may show a dimmed suggested prompt in an empty input box; it is not typed text and " +
			"typing replaces it. noSubmit types without pressing Enter.",
	}, ctrl.SendPrompt)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "send_keys",
		Title: "Press keys in a session",
		Description: "Press tmux named keys in a session's agent pane, in order, and return the screen after: Enter, " +
			"Escape, Up/Down to move a selection, digits for numbered options, C-u to clear the input line, C-c to " +
			"interrupt.",
	}, ctrl.SendKeys)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "wait_for_session",
		Title: "Wait for a session's status",
		Description: "Block until a session's agent status is one of states (active, approval, ready, missing), or " +
			"until it changes from what it is now when states is empty, or until timeoutSeconds passes (default 50, " +
			"at most 240). Prefer this to polling peek_session: it costs one call, not one per check. On timedOut, " +
			"call again to keep waiting.",
	}, ctrl.WaitForSession)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "publish_message",
		Title: "Publish to the message bus",
		Description: "Publish a payload to a topic on hive's message bus, which hive CLI sessions read with hive msg. " +
			"A session's inbox is agent.<session-id>.inbox. A topic with a * reaches every existing topic it matches. " +
			"Agents do not poll their inbox: after publishing, tell the session to read it with send_prompt.",
	}, ctrl.PublishMessage)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "wait_for_message",
		Title: "Wait for bus messages",
		Description: "Block until a topic holds messages this workspace has not taken yet, then take and return them; " +
			"each message is handed back once. A topic with a * waits on every existing topic it matches. Answers " +
			"timedOut after timeoutSeconds (default 50, at most 240) with nothing taken; call again to keep waiting.",
	}, ctrl.WaitForMessage)

	mcp.AddTool(srv, &mcp.Tool{
		Name:  "sleep",
		Title: "Sleep",
		Description: "Block for seconds (at most 240) and return. Use it to check back on a session later without a " +
			"polling loop; prefer wait_for_session or wait_for_message when there is something specific to wait for.",
	}, ctrl.Sleep)
}

type repositoriesResult struct {
	Repositories      []repositoryView `json:"repositories"`
	DefaultRepository string           `json:"defaultRepository,omitempty" jsonschema:"The repository hive's own UI preselects. start_session still needs one named."`
	Agents            []string         `json:"agents"                      jsonschema:"Agent profiles hive can launch."`
	DefaultAgent      string           `json:"defaultAgent,omitempty"`
}

type repositoryView struct {
	Name   string `json:"name"`
	Remote string `json:"remote"`
}

func (ctrl *OrchestratorController) ListRepositories(ctx context.Context, req *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, repositoriesResult, error) {
	caller, err := ctrl.caller(ctx, req)
	if err != nil {
		return nil, repositoriesResult{}, err
	}
	opts, err := ctrl.core.Orchestration.Repositories(ctx, caller)
	if err != nil {
		return nil, repositoriesResult{}, ctrl.toolError(err)
	}
	out := repositoriesResult{Repositories: []repositoryView{}, Agents: opts.Agents, DefaultRepository: opts.DefaultRepository, DefaultAgent: opts.DefaultAgent}
	if out.Agents == nil {
		out.Agents = []string{}
	}
	for _, r := range opts.Repositories {
		out.Repositories = append(out.Repositories, repositoryView{Name: r.Name, Remote: r.Repository})
	}
	return nil, out, nil
}

type startSessionInput struct {
	Repository string   `json:"repository"      jsonschema:"The repository's remote URL or its name from list_repositories."`
	Name       string   `json:"name"            jsonschema:"The session name, unique among sessions. It names the checkout directory and the tmux session; the branch follows the hive config."`
	Prompt     string   `json:"prompt"          jsonschema:"The agent's first message: what to do, the deliverable, and when to stop."`
	Agent      string   `json:"agent,omitempty" jsonschema:"An agent profile from list_repositories. Omit for the default."`
	Tags       []string `json:"tags,omitempty"  jsonschema:"Extra labels beside orchestrator, to group sessions for list_sessions."`
}

type startedSession struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Path string `json:"path" jsonschema:"The checkout on disk. Read its .hive/ directory and git log from here to see progress."`
}

func (ctrl *OrchestratorController) StartSession(ctx context.Context, req *mcp.CallToolRequest, in startSessionInput) (*mcp.CallToolResult, startedSession, error) {
	caller, err := ctrl.caller(ctx, req)
	if err != nil {
		return nil, startedSession{}, err
	}
	out, err := ctrl.core.Orchestration.StartSession(ctx, caller, app.OrchestratedSessionRequest{
		Repository: in.Repository, Name: in.Name, Prompt: in.Prompt, Agent: in.Agent, Tags: in.Tags,
	})
	if err != nil {
		return nil, startedSession{}, ctrl.toolError(err)
	}
	return nil, startedSession{ID: out.ID, Name: out.Name, Slug: out.Slug, Path: out.Path}, nil
}

type listSessionsInput struct {
	All  bool     `json:"all,omitempty"  jsonschema:"List every session in every state, not only active ones tagged orchestrator."`
	Tags []string `json:"tags,omitempty" jsonschema:"Only sessions carrying every one of these tags."`
}

type sessionView struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Repository  string   `json:"repository"`
	Path        string   `json:"path"`
	State       string   `json:"state"                 jsonschema:"active, recycled or corrupted: the session's lifecycle, not its agent's."`
	Tags        []string `json:"tags"`
	Running     bool     `json:"running"               jsonschema:"Whether its tmux session is alive."`
	AgentStatus string   `json:"agentStatus,omitempty" jsonschema:"active, approval, ready or missing, as last published. Empty when not running."`
	CreatedAt   string   `json:"createdAt"`
}

type sessionsResult struct {
	Sessions []sessionView `json:"sessions"`
}

func (ctrl *OrchestratorController) ListSessions(ctx context.Context, req *mcp.CallToolRequest, in listSessionsInput) (*mcp.CallToolResult, sessionsResult, error) {
	caller, err := ctrl.caller(ctx, req)
	if err != nil {
		return nil, sessionsResult{}, err
	}
	sessions, err := ctrl.core.Orchestration.Sessions(ctx, caller, in.All, in.Tags)
	if err != nil {
		return nil, sessionsResult{}, ctrl.toolError(err)
	}
	out := sessionsResult{Sessions: make([]sessionView, len(sessions))}
	for i, s := range sessions {
		out.Sessions[i] = sessionViewOf(s)
	}
	return nil, out, nil
}

func sessionViewOf(s app.OrchestratedSession) sessionView {
	tags := s.Tags
	if tags == nil {
		tags = []string{}
	}
	return sessionView{
		ID: s.ID, Name: s.Name, Repository: s.Remote, Path: s.Path, State: string(s.State), Tags: tags,
		Running: s.Running, AgentStatus: s.AgentStatus, CreatedAt: s.CreatedAt.UTC().Format(time.RFC3339),
	}
}

type peekSessionInput struct {
	Session string `json:"session"         jsonschema:"The session id from start_session or list_sessions."`
	Lines   int    `json:"lines,omitempty" jsonschema:"Screen lines to return in tail. Default 30."`
}

type contextUsageView struct {
	Used    string `json:"used"`
	Total   string `json:"total"`
	Percent int    `json:"percent"`
}

type peekView struct {
	State   string            `json:"state"             jsonschema:"A hint: active, approval, question, ready, unknown or missing."`
	Tool    string            `json:"tool,omitempty"`
	Context *contextUsageView `json:"context,omitempty" jsonschema:"Context window use, when the agent's status bar shows it."`
	Tail    string            `json:"tail"              jsonschema:"The last lines of the agent's screen."`
}

func peekViewOf(peek prompt.SessionPeek) peekView {
	out := peekView{State: peek.State.String(), Tool: peek.Tool, Tail: peek.Tail}
	if peek.Context != nil {
		out.Context = &contextUsageView{Used: peek.Context.Used, Total: peek.Context.Total, Percent: peek.Context.Percent}
	}
	return out
}

func (ctrl *OrchestratorController) PeekSession(ctx context.Context, req *mcp.CallToolRequest, in peekSessionInput) (*mcp.CallToolResult, peekView, error) {
	caller, err := ctrl.caller(ctx, req)
	if err != nil {
		return nil, peekView{}, err
	}
	peek, err := ctrl.core.Orchestration.Peek(ctx, caller, in.Session, screenLines(in.Lines))
	if err != nil {
		return nil, peekView{}, ctrl.toolError(err)
	}
	return nil, peekViewOf(peek), nil
}

func screenLines(n int) int {
	if n <= 0 {
		return 30
	}
	return n
}

type sendPromptInput struct {
	Session  string `json:"session"            jsonschema:"The session id."`
	Text     string `json:"text"               jsonschema:"What to type. Several lines are pasted as one message."`
	NoSubmit bool   `json:"noSubmit,omitempty" jsonschema:"Type the text without pressing Enter."`
	Lines    int    `json:"lines,omitempty"    jsonschema:"Screen lines to return. Default 30."`
}

func (ctrl *OrchestratorController) SendPrompt(ctx context.Context, req *mcp.CallToolRequest, in sendPromptInput) (*mcp.CallToolResult, peekView, error) {
	caller, err := ctrl.caller(ctx, req)
	if err != nil {
		return nil, peekView{}, err
	}
	peek, err := ctrl.core.Orchestration.SendPrompt(ctx, caller, app.PromptSend{
		SessionID: in.Session, Text: in.Text, NoSubmit: in.NoSubmit, Lines: screenLines(in.Lines),
	})
	if err != nil {
		return nil, peekView{}, ctrl.toolError(err)
	}
	return nil, peekViewOf(peek), nil
}

type sendKeysInput struct {
	Session string   `json:"session"         jsonschema:"The session id."`
	Keys    []string `json:"keys"            jsonschema:"tmux key names pressed in order, such as Enter, Escape, Down, 1, C-c."`
	Lines   int      `json:"lines,omitempty" jsonschema:"Screen lines to return. Default 30."`
}

func (ctrl *OrchestratorController) SendKeys(ctx context.Context, req *mcp.CallToolRequest, in sendKeysInput) (*mcp.CallToolResult, peekView, error) {
	caller, err := ctrl.caller(ctx, req)
	if err != nil {
		return nil, peekView{}, err
	}
	peek, err := ctrl.core.Orchestration.SendKeys(ctx, caller, in.Session, in.Keys, screenLines(in.Lines))
	if err != nil {
		return nil, peekView{}, ctrl.toolError(err)
	}
	return nil, peekViewOf(peek), nil
}

type waitForSessionInput struct {
	Session        string   `json:"session"                  jsonschema:"The session id."`
	States         []string `json:"states,omitempty"         jsonschema:"Statuses to wait for: active, approval, ready, missing. Empty waits for any change."`
	TimeoutSeconds int      `json:"timeoutSeconds,omitempty" jsonschema:"How long to wait. Default 50, at most 240."`
}

type waitForSessionResult struct {
	Matched       bool        `json:"matched"`
	TimedOut      bool        `json:"timedOut"`
	WaitedSeconds float64     `json:"waitedSeconds"`
	Session       sessionView `json:"session"`
}

func (ctrl *OrchestratorController) WaitForSession(ctx context.Context, req *mcp.CallToolRequest, in waitForSessionInput) (*mcp.CallToolResult, waitForSessionResult, error) {
	caller, err := ctrl.caller(ctx, req)
	if err != nil {
		return nil, waitForSessionResult{}, err
	}
	res, err := ctrl.core.Orchestration.WaitForSession(ctx, caller, in.Session, in.States, seconds(in.TimeoutSeconds))
	if err != nil {
		return nil, waitForSessionResult{}, ctrl.toolError(err)
	}
	return nil, waitForSessionResult{
		Matched: res.Matched, TimedOut: res.TimedOut, WaitedSeconds: res.Waited.Seconds(), Session: sessionViewOf(res.Session),
	}, nil
}

type publishMessageInput struct {
	Topic   string `json:"topic"   jsonschema:"The topic, such as agent.<session-id>.inbox. A * matches existing topics."`
	Payload string `json:"payload" jsonschema:"The message body. JSON is conventional for structured data."`
}

type publishMessageResult struct {
	Topics []string `json:"topics" jsonschema:"The topics the message reached."`
}

func (ctrl *OrchestratorController) PublishMessage(ctx context.Context, req *mcp.CallToolRequest, in publishMessageInput) (*mcp.CallToolResult, publishMessageResult, error) {
	caller, err := ctrl.caller(ctx, req)
	if err != nil {
		return nil, publishMessageResult{}, err
	}
	topics, err := ctrl.core.Orchestration.Publish(ctx, caller, in.Topic, in.Payload)
	if err != nil {
		return nil, publishMessageResult{}, ctrl.toolError(err)
	}
	if topics == nil {
		topics = []string{}
	}
	return nil, publishMessageResult{Topics: topics}, nil
}

type waitForMessageInput struct {
	Topic          string `json:"topic"                    jsonschema:"The topic to wait on. A * waits on every existing topic it matches."`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty" jsonschema:"How long to wait. Default 50, at most 240."`
}

type busMessageView struct {
	ID        string `json:"id"`
	Topic     string `json:"topic"`
	Payload   string `json:"payload"`
	Sender    string `json:"sender,omitempty"`
	SessionID string `json:"sessionId,omitempty" jsonschema:"The hive session that sent it, when it was sent from one."`
	CreatedAt string `json:"createdAt"`
}

type waitForMessageResult struct {
	Messages      []busMessageView `json:"messages"`
	TimedOut      bool             `json:"timedOut"`
	WaitedSeconds float64          `json:"waitedSeconds"`
}

func (ctrl *OrchestratorController) WaitForMessage(ctx context.Context, req *mcp.CallToolRequest, in waitForMessageInput) (*mcp.CallToolResult, waitForMessageResult, error) {
	caller, err := ctrl.caller(ctx, req)
	if err != nil {
		return nil, waitForMessageResult{}, err
	}
	res, err := ctrl.core.Orchestration.WaitForMessages(ctx, caller, in.Topic, seconds(in.TimeoutSeconds))
	if err != nil {
		return nil, waitForMessageResult{}, ctrl.toolError(err)
	}
	out := waitForMessageResult{Messages: make([]busMessageView, len(res.Messages)), TimedOut: res.TimedOut, WaitedSeconds: res.Waited.Seconds()}
	for i, m := range res.Messages {
		out.Messages[i] = busMessageView{
			ID: m.ID, Topic: m.Topic, Payload: m.Payload, Sender: m.Sender, SessionID: m.SessionID,
			CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339),
		}
	}
	return nil, out, nil
}

type sleepInput struct {
	Seconds int `json:"seconds" jsonschema:"How long to sleep, at most 240."`
}

type sleepResult struct {
	SleptSeconds float64 `json:"sleptSeconds"`
}

func (ctrl *OrchestratorController) Sleep(ctx context.Context, req *mcp.CallToolRequest, in sleepInput) (*mcp.CallToolResult, sleepResult, error) {
	caller, err := ctrl.caller(ctx, req)
	if err != nil {
		return nil, sleepResult{}, err
	}
	slept, err := ctrl.core.Orchestration.Sleep(ctx, caller, seconds(in.Seconds))
	if err != nil {
		return nil, sleepResult{}, ctrl.toolError(err)
	}
	return nil, sleepResult{SleptSeconds: slept.Seconds()}, nil
}

func seconds(n int) time.Duration { return time.Duration(n) * time.Second }
