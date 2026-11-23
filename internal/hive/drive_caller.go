package hive

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/colonyops/hive/internal/domain/drive"
	drivesvc "github.com/colonyops/hive/internal/hive/drive"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/ops"
)

// DriveCaller resolves a caller value to the Drive space its documents belong
// to: a hive session's repository, a chat's workspace, or the shared space for
// global. A path inside no session is the repository of its git remote, the
// same space the sessions of that repository use.
func (e *Engine) DriveCaller(ctx context.Context, value string) (drivesvc.Caller, error) {
	c, err := e.Sessions().ResolveCaller(ctx, value, "", "")
	if err != nil {
		dir := strings.TrimSpace(value)
		if opErr, ok := errors.AsType[*ops.Error](err); ok && opErr.Kind == ops.KindNotFound && filepath.IsAbs(dir) {
			return e.dirDriveCaller(ctx, dir, opErr)
		}
		return drivesvc.Caller{}, err
	}
	switch c.Kind {
	case sessionsvc.CallerKindGlobal:
		return drivesvc.Caller{Space: drive.Shared}, nil
	case sessionsvc.CallerKindSession:
		return e.drive.SessionCaller(c.Session)
	case sessionsvc.CallerKindChat:
		if e.ports.Chats == nil {
			return drivesvc.Caller{}, ops.Errorf(ops.KindUnavailable, "%q is a chat; its documents need Hive Desktop running", c.ChatID)
		}
		chat, err := e.ports.Chats.Chat(ctx, c.ChatID)
		if err != nil {
			if opErr, ok := errors.AsType[*ops.Error](err); ok {
				return drivesvc.Caller{}, opErr
			}
			return drivesvc.Caller{}, ops.Errorf(ops.KindInternal, "loading session %q: %v", c.ChatID, err)
		}
		return drivesvc.ChatCaller(chat)
	}
	return drivesvc.Caller{}, ops.Errorf(ops.KindInternal, "caller %q resolved to an unknown kind %q", value, c.Kind)
}

// dirDriveCaller is a directory inside no session. One with no remote that
// names an owner and repository keeps notFound: nothing says where its
// documents belong.
func (e *Engine) dirDriveCaller(ctx context.Context, dir string, notFound *ops.Error) (drivesvc.Caller, error) {
	remote, err := e.Git().RemoteURL(ctx, dir)
	if err != nil {
		e.ports.Logger.Debug().Err(err).Str("dir", dir).Msg("no git remote for the drive caller's directory")
		return drivesvc.Caller{}, notFound
	}
	space, ok := e.drive.SpaceForRemote(remote)
	if !ok {
		return drivesvc.Caller{}, notFound
	}
	return drivesvc.Caller{Space: space}, nil
}
