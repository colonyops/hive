package app

import (
	"context"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	statussvc "github.com/colonyops/hive/internal/hive/status"
)

// fleetAgents is serverAgents on the server fleet: the daemon of each server
// finds its agents, and the server's tmux manager, the one its terminals run
// on, types into them.
type fleetAgents struct {
	api       func(server string) (sessionapi.API, error)
	terminals *serverTerminals
}

// Each call gets its own serverReadTimeout: the first can pay for the ssh
// handshake, and AgentPanes captures every pane of the session on the server.
func (f fleetAgents) SessionAgentPanes(ctx context.Context, server, id string) (session.Session, []statussvc.AgentPane, error) {
	api, err := f.api(server)
	if err != nil {
		return session.Session{}, nil, err
	}
	getCtx, cancel := context.WithTimeout(ctx, serverReadTimeout)
	defer cancel()
	sess, err := api.GetSession(getCtx, id)
	if err != nil {
		return session.Session{}, nil, serverError(err, kindOfRead(err), "reading session %q on server %q", id, server)
	}
	panesCtx, cancel := context.WithTimeout(ctx, serverReadTimeout)
	defer cancel()
	panes, err := api.AgentPanes(panesCtx, []string{id})
	if err != nil {
		return session.Session{}, nil, serverError(err, KindUnavailable, "listing the agents of session %q on server %q", sess.Name, server)
	}
	return sess, panes[id], nil
}

func (f fleetAgents) Input(ctx context.Context, server string) (agentInput, error) {
	return f.terminals.Manager(ctx, server)
}
