package app

import (
	"context"
	"fmt"
	"os"

	"github.com/colonyops/hive/cmd/hive/internal/serverattach"
	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
)

// StartServerSession makes sure the tmux session of sess on the named server
// runs, by the server's rules, and returns the ssh target to attach with.
func (a *App) StartServerSession(ctx context.Context, serverName string, sess session.Session) (server.Host, error) {
	srv, ok := a.Servers.Server(serverName)
	if !ok {
		return server.Host{}, fmt.Errorf("no server named %q under servers: in config.yaml", serverName)
	}
	if _, err := srv.Client().StartTmuxSession(ctx, sess.ID); err != nil {
		return server.Host{}, fmt.Errorf("start session %s on server %s: %w", sess.Name, serverName, err)
	}
	return srv.Config().Host(), nil
}

// AttachServerSession starts sess on the named server and attaches to it:
// in a new window inside tmux, so the caller keeps its own, and on this
// terminal outside it.
func (a *App) AttachServerSession(ctx context.Context, serverName string, sess session.Session) error {
	host, err := a.StartServerSession(ctx, serverName, sess)
	if err != nil {
		return err
	}
	tmuxSession := sessionsvc.Target(sess).Session
	if os.Getenv("TMUX") != "" {
		return serverattach.OpenWindow(ctx, host, tmuxSession, serverName+"/"+sess.Name)
	}
	return serverattach.Run(ctx, host, tmuxSession)
}
