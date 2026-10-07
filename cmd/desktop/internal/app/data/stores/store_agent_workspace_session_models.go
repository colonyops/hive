package stores

import (
	"strconv"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

// AgentSession is one durable record of an agent workspace session: the
// launch table entry a terminal is resolved from and reattached through.
type AgentSession struct {
	ID             string `json:"id"`
	LegacyID       string `json:"legacyId"`
	Workspace      string `json:"workspace"`
	Name           string `json:"name"`
	Agent          string `json:"agent"`
	AgentSessionID string `json:"agentSessionId"`
	// TerminalID is the immutable random suffix in this chat's tmux session
	// name. It is independent of the database id because several isolated
	// databases can share one tmux namespace.
	TerminalID   string `json:"terminalId"`
	CreatedAt    int64  `json:"createdAt"`
	LastOpenedAt int64  `json:"lastOpenedAt"`
	// ScheduleID names the schedule that started this chat, empty for one
	// started by hand. It is a column rather than a lookup in run history,
	// which can be pruned independently of the chat.
	ScheduleID string `json:"scheduleId"`
	// EndToken is the capability a launch hands its own process; presenting it
	// is what lets a chat end its own session. It never leaves the core.
	EndToken string `json:"-"`
}

// AgentSessionCreate is Create's input: ID, CreatedAt and LastOpenedAt are
// assigned by the store.
type AgentSessionCreate struct {
	Workspace      string
	Name           string
	Agent          string
	AgentSessionID string
	TerminalID     string
	ScheduleID     string
	EndToken       string
}

func mapAgentSessionFromDB(row queries.AgentWorkspaceSession) AgentSession {
	legacyID := ""
	if row.LegacyID.Valid {
		legacyID = strconv.FormatInt(row.LegacyID.Int64, 10)
	}
	return AgentSession{
		ID: row.ID, LegacyID: legacyID, Workspace: row.Workspace, Name: row.Name, Agent: row.Agent,
		AgentSessionID: row.AgentSessionID, TerminalID: row.TerminalID,
		CreatedAt: row.CreatedAt, LastOpenedAt: row.LastOpenedAt,
		ScheduleID: row.ScheduleID, EndToken: row.EndToken,
	}
}
