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

func (f fleetAgents) SessionAgentPanes(ctx context.Context, server, id string) (session.Session, []statussvc.AgentPane, error) {
	api, err := f.api(server)
	if err != nil {
		return session.Session{}, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, serverReadTimeout)
	defer cancel()
	sess, err := api.GetSession(ctx, id)
	if err != nil {
		return session.Session{}, nil, serverError(err, kindOfRead(err), "reading session %q on server %q", id, server)
	}
	panes, err := api.AgentPanes(ctx, []string{id})
	if err != nil {
		return session.Session{}, nil, serverError(err, KindUnavailable, "listing the agents of session %q on server %q", sess.Name, server)
	}
	return sess, panes[id], nil
}

func (f fleetAgents) Input(ctx context.Context, server string) (agentInput, error) {
	return f.terminals.Manager(ctx, server)
}
