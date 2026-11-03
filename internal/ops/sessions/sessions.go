// Package sessions declares the hive session operations of the desktop's
// app-control MCP server.
package sessions

import (
	"errors"

	"github.com/colonyops/hive/internal/ops"
)

var Toolset = ops.Toolset{Name: "sessions", MCPPath: "/mcp"}

func All() []ops.Op {
	return []ops.Op{SessionList, SessionTagsUpdate}
}

type SessionListIn struct {
	Tags  []string `json:"tags,omitempty"  jsonschema:"Only sessions carrying every one of these tags. Tags compare case-insensitively."`
	State string   `json:"state,omitempty" jsonschema:"Only sessions in this state: active, recycled or corrupted."`
}

var SessionList = ops.Spec[SessionListIn]{
	Name: "session.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "List hive sessions",
	Guidance: "List hive sessions newest first, with each one's tags and state; slug is the tmux session name an attach targets. " +
		"tags keeps only sessions that carry every tag given, compared case-insensitively, so polling with a tag such as auto finds the sessions waiting for an agent to pick them up. " +
		"state keeps only active, recycled or corrupted sessions. An empty list means no session matched, not that hive is unreachable: a failed read is an error.",
	Fields: ops.Constraints{"state": {Enum: []string{"active", "recycled", "corrupted"}}},
}

type SessionTagsUpdateIn struct {
	SessionID string   `json:"sessionId"        jsonschema:"Session id, from session_list."`
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
	Name: "session_tags.update", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary: "Add and remove a session's tags",
	Guidance: "Add tags to one hive session and then remove tags from it, leaving its other tags in place, and return the session. " +
		"Use one call to swap a tag, for example remove auto and add claimed when an orchestrator picks a session up; two agents that each add a tag both keep theirs. " +
		"Tags compare case-insensitively, keep the first spelling, are at most 40 characters and cannot contain commas. " +
		"This changes the session record only: it starts, stops and attaches nothing.",
	Args: []string{"sessionId"},
}
