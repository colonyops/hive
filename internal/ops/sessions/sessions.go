// Package sessions declares the hive session operations. They run on the
// engine and reach every session the server fleet holds: this machine's and
// those of each server under servers: in config.yaml.
package sessions

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/colonyops/hive/internal/ops"
)

var Toolset = ops.Toolset{Name: "sessions", MCPPath: "/mcp"}

// ControlToolset holds the operations that start a process or destroy a
// checkout (create, start, recycle, delete), so they are served only on the
// token-guarded server (ADR
// chat-control-is-served-on-a-token-guarded-mcp-server-whose-token-the-endpoint-file-carries).
var ControlToolset = ops.Toolset{Name: "session-control", MCPPath: "/mcp/sessions"}

func All() []ops.Op {
	return []ops.Op{
		SessionList, SessionGet, SessionUpdate, SessionTagsUpdate, SessionDelete, SessionRecycle,
		SessionCreate, SessionStart,
	}
}

// States are the session states a list can filter by.
var States = []string{"active", "recycled", "corrupted"}

const (
	refGuidance        = "A sessionId is a bare id on server (default local) or <server>:<id>."
	riskGuidance       = "It refuses with conflict when the session has uncommitted changes or unpushed commits, unless force is set. For a worktree session the check can report a pushed but unmerged branch as unpushed."
	tagRulesGuidance   = "Tags compare case-insensitively, keep the first spelling, are at most 40 characters and cannot contain commas."
	sessionAnswerShape = "Answers with the session: server, id, name, slug, repo (the repository name), remote, path, inbox, state, group, clone_strategy, tags, created_at and updated_at."
)

type SessionListIn struct {
	Tags   []string `json:"tags,omitempty"   jsonschema:"Only sessions carrying every one of these tags. Tags compare case-insensitively."`
	State  string   `json:"state,omitempty"  jsonschema:"Only sessions in this state. Without it the list holds active and recycled sessions, and corrupted ones are reported apart."`
	Server string   `json:"server,omitempty" jsonschema:"Only the sessions of this server under servers: in config.yaml; local is this machine. Without it the list holds this machine and every server."`
}

var SessionList = ops.Spec[SessionListIn]{
	Name: "session.list", Owner: ops.OwnerEngine, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "List hive sessions on this machine and every server",
	Guidance: "List hive sessions: this machine's first, then each server's under servers: in config.yaml in config order, each sorted by repository. " +
		"Each session names its server; slug is the tmux session name an attach targets, and inbox and unread are given for this machine's sessions only. " +
		"tags keeps only sessions that carry every tag given, compared case-insensitively, so polling with a tag such as auto finds the sessions waiting for an agent to pick them up. " +
		"state keeps only active, recycled or corrupted sessions; without it, corrupted sessions are listed apart under corrupted. " +
		"A server that does not answer is listed under offline with its error and does not fail the call; this machine, or the one server named, failing is an error. " +
		"An empty list means no session matched.",
	Fields: ops.Constraints{"state": {Enum: States}},
}

type SessionGetIn struct {
	SessionID string `json:"sessionId"        jsonschema:"The session id from session_list, or <server>:<id> for a session on a server."`
	Server    string `json:"server,omitempty" jsonschema:"The server under servers: in config.yaml that holds the session: local (default) for this machine. A sessionId of the form <server>:<id> wins over it."`
}

var SessionGet = ops.Spec[SessionGetIn]{
	Name: "session.get", Owner: ops.OwnerEngine, Effect: ops.EffectRead, Toolset: Toolset,
	Summary:  "Show one hive session",
	Guidance: refGuidance + " " + sessionAnswerShape,
	Args:     []string{"sessionId"},
}

type SessionUpdateIn struct {
	SessionID  string   `json:"sessionId"            jsonschema:"The session id from session_list, or <server>:<id> for a session on a server."`
	Server     string   `json:"server,omitempty"     jsonschema:"The server under servers: in config.yaml that holds the session: local (default) for this machine. A sessionId of the form <server>:<id> wins over it."`
	Name       string   `json:"name,omitempty"       jsonschema:"A new name; the slug follows it."`
	Group      string   `json:"group,omitempty"      jsonschema:"Put the session in this group."`
	ClearGroup bool     `json:"clearGroup,omitempty" jsonschema:"Take the session out of its group."`
	AddTag     []string `json:"addTag,omitempty"     jsonschema:"Tags to add."`
	RemoveTag  []string `json:"removeTag,omitempty"  jsonschema:"Tags to remove, after the adds."`
	ClearTags  bool     `json:"clearTags,omitempty"  jsonschema:"Remove every tag before the adds."`
}

func (in SessionUpdateIn) Validate() error {
	if in.Group != "" && in.ClearGroup {
		return errors.New("group and clearGroup cannot be combined")
	}
	if in.Name == "" && in.Group == "" && !in.ClearGroup && len(in.AddTag) == 0 && len(in.RemoveTag) == 0 && !in.ClearTags {
		return errors.New("nothing to update: provide name, group, clearGroup, addTag, removeTag or clearTags")
	}
	return nil
}

var SessionUpdate = ops.Spec[SessionUpdateIn]{
	Name: "session.update", Owner: ops.OwnerEngine, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary: "Rename a session, change its group or edit its tags",
	Guidance: "Change a session's name, group or tags and answer with the session. " + refGuidance + " " +
		"Tag edits apply in order: clearTags, then addTag, then removeTag. " + tagRulesGuidance + " " +
		"This changes the session record only: it starts, stops and attaches nothing.",
	Args: []string{"sessionId"},
}

type SessionTagsUpdateIn struct {
	SessionID string   `json:"sessionId"        jsonschema:"The session id from session_list, or <server>:<id> for a session on a server."`
	Server    string   `json:"server,omitempty" jsonschema:"The server under servers: in config.yaml that holds the session: local (default) for this machine. A sessionId of the form <server>:<id> wins over it."`
	Add       []string `json:"add,omitempty"    jsonschema:"Tags to add. A tag the session already carries is kept as it is."`
	Remove    []string `json:"remove,omitempty" jsonschema:"Tags to remove, after the adds. A tag the session does not carry is ignored."`
}

func (in SessionTagsUpdateIn) Validate() error {
	if len(in.Add) == 0 && len(in.Remove) == 0 {
		return errors.New("provide add or remove")
	}
	return nil
}

var SessionTagsUpdate = ops.Spec[SessionTagsUpdateIn]{
	Name: "session_tags.update", Owner: ops.OwnerEngine, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary: "Add and remove a session's tags",
	Guidance: "Add tags to one hive session and then remove tags from it, leaving its other tags in place, and answer with the session. " + refGuidance + " " +
		"Use one call to swap a tag, for example remove auto and add claimed when an orchestrator picks a session up; two agents that each add a tag both keep theirs. " +
		tagRulesGuidance + " This changes the session record only: it starts, stops and attaches nothing.",
	Args: []string{"sessionId"},
}

type SessionDeleteIn struct {
	SessionID string `json:"sessionId"        jsonschema:"The session id from session_list, or <server>:<id> for a session on a server."`
	Server    string `json:"server,omitempty" jsonschema:"The server under servers: in config.yaml that holds the session: local (default) for this machine. A sessionId of the form <server>:<id> wins over it."`
	Force     bool   `json:"force,omitempty"  jsonschema:"Delete even when the session has uncommitted changes or unpushed commits."`
}

var SessionDelete = ops.Spec[SessionDeleteIn]{
	Name: "session.delete", Owner: ops.OwnerEngine, Effect: ops.EffectWrite, Toolset: ControlToolset,
	Summary: "Delete a session and its directory",
	Guidance: "Remove a session, its clone and its tmux session for good, and answer with its server, id and deleted. " + refGuidance + " " +
		riskGuidance + " To keep the clone for reuse, use session_recycle.",
	Args: []string{"sessionId"},
}

type SessionRecycleIn struct {
	SessionID string `json:"sessionId"        jsonschema:"The session id from session_list, or <server>:<id> for a session on a server."`
	Server    string `json:"server,omitempty" jsonschema:"The server under servers: in config.yaml that holds the session: local (default) for this machine. A sessionId of the form <server>:<id> wins over it."`
	Force     bool   `json:"force,omitempty"  jsonschema:"Recycle even when the session has uncommitted changes or unpushed commits."`
}

var SessionRecycle = ops.Spec[SessionRecycleIn]{
	Name: "session.recycle", Owner: ops.OwnerEngine, Effect: ops.EffectWrite, Toolset: ControlToolset,
	Summary: "Recycle a session so its clone can be reused",
	Guidance: "Reset a full-clone session's checkout so a new session can reuse it, and answer with the recycled session. " +
		"A worktree session loses its checkout and record and keeps the shared bare clone; the answer is then its server, id and deleted. " +
		refGuidance + " " + riskGuidance,
	Args: []string{"sessionId"},
}

type SessionCreateIn struct {
	Caller        string   `json:"caller"                  jsonschema:"Who is calling. In a hive session it is the absolute path of your working directory; outside one it is that directory too. The source directory and the remote default to it."`
	Name          string   `json:"name"                    jsonschema:"The session's name."`
	Server        string   `json:"server,omitempty"        jsonschema:"The server under servers: in config.yaml to create the session on: local (default) for this machine."`
	Remote        string   `json:"remote,omitempty"        jsonschema:"The git remote URL to clone. Defaults to the origin of the caller's directory."`
	Source        string   `json:"source,omitempty"        jsonschema:"An absolute directory whose files copy rules read. Defaults to the caller's directory. A server cannot read it, so a create on a server takes none."`
	Background    bool     `json:"background,omitempty"    jsonschema:"Do not attach a terminal to the new session. An MCP call always creates in the background."`
	CloneStrategy string   `json:"cloneStrategy,omitempty" jsonschema:"full or worktree. Defaults to the repository's rule."`
	Agent         string   `json:"agent,omitempty"         jsonschema:"The agent profile to start, by its key under agents: on the machine that holds the session."`
	Tags          []string `json:"tags,omitempty"          jsonschema:"Tags for the new session."`
}

var SessionCreate = ops.Spec[SessionCreateIn]{
	Name: "session.create", Owner: ops.OwnerEngine, Effect: ops.EffectExec, Toolset: ControlToolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Create a hive session and start its agent",
	Guidance: "Clone the repository, or reuse a recycled clone of it, on this machine or a server, run the repository's rules and start the agent in a tmux session in the background. " +
		"This runs commands from config.yaml and the agent profile. " + tagRulesGuidance + " " + sessionAnswerShape,
	Args:   []string{"name"},
	Fields: ops.Constraints{"cloneStrategy": {Enum: []string{"full", "worktree"}}},
}

type SessionStartIn struct {
	SessionID string `json:"sessionId"        jsonschema:"The session id from session_list, or <server>:<id> for a session on a server."`
	Server    string `json:"server,omitempty" jsonschema:"The server under servers: in config.yaml that holds the session: local (default) for this machine. A sessionId of the form <server>:<id> wins over it."`
}

var SessionStart = ops.Spec[SessionStartIn]{
	Name: "session.start", Owner: ops.OwnerEngine, Effect: ops.EffectExec, Toolset: ControlToolset,
	Summary: "Start an active session's tmux session and agent",
	Guidance: "Open the tmux session of an active session in the background on the machine that holds it, and start its agent when no tmux session runs. " + refGuidance + " " +
		"Answers with server, id, created (a new tmux session started) and completed (every command of the session already finished, so no terminal runs).",
	Args: []string{"sessionId"},
}

// SessionOut is a session as every surface prints it: the JSON hive session
// show --json printed, plus the fields only a list fills.
type SessionOut struct {
	Server        string    `json:"server"`
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	Repo          string    `json:"repo"`
	Remote        string    `json:"remote"`
	Path          string    `json:"path"`
	Inbox         string    `json:"inbox,omitempty"`
	State         string    `json:"state"`
	Group         string    `json:"group,omitempty"`
	CloneStrategy string    `json:"clone_strategy,omitempty"`
	Tags          []string  `json:"tags"`
	Unread        *int      `json:"unread,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ServerFailure is a server whose sessions could not be read.
type ServerFailure struct {
	Server string `json:"server"`
	Error  string `json:"error"`
}

type SessionListOut struct {
	Sessions  []SessionOut    `json:"sessions"`
	Corrupted []SessionOut    `json:"corrupted,omitempty"`
	Offline   []ServerFailure `json:"offline,omitempty"`
}

// JSONLines prints one session per line, as hive session list --json did.
func (o SessionListOut) JSONLines() []any {
	out := make([]any, len(o.Sessions))
	for i, s := range o.Sessions {
		out[i] = s
	}
	return out
}

type SessionDeletedOut struct {
	Server  string `json:"server"`
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// SessionRecycleOut is the recycled session, or Deleted when the recycle
// removed the record, as it does for a worktree session.
type SessionRecycleOut struct {
	Session *SessionOut
	Deleted *SessionDeletedOut
}

func (o SessionRecycleOut) MarshalJSON() ([]byte, error) {
	if o.Deleted != nil {
		return json.Marshal(o.Deleted)
	}
	return json.Marshal(o.Session)
}

// OutputSchema is one of the two shapes MarshalJSON writes.
func (SessionRecycleOut) OutputSchema() (*jsonschema.Schema, error) {
	kept, err := jsonschema.For[SessionOut](nil)
	if err != nil {
		return nil, err
	}
	deleted, err := jsonschema.For[SessionDeletedOut](nil)
	if err != nil {
		return nil, err
	}
	return &jsonschema.Schema{Type: "object", OneOf: []*jsonschema.Schema{kept, deleted}}, nil
}

type SessionStartOut struct {
	Server    string `json:"server"`
	ID        string `json:"id"`
	Created   bool   `json:"created"`
	Completed bool   `json:"completed"`
}
