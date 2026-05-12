//go:build windows

package timer

import (
	"context"
	"database/sql"
	"errors"
)

type Row struct {
	ID  string
	Pid sql.NullInt64
}

type MarkInactiveParams struct {
	SessionID string
	IDs       []string
}

type InactiveQuerier interface {
	ActiveTimersForSession(ctx context.Context, sessionID string) ([]Row, error)
	ActiveTimersAll(ctx context.Context) ([]Row, error)
	MarkInactiveTimersForSession(ctx context.Context, arg MarkInactiveParams) error
	MarkInactiveTimersAll(ctx context.Context, ids []string) error
}

func MarkInactiveForSession(ctx context.Context, q InactiveQuerier, sessionID string) (int, error) {
	return 0, errors.New("timer: MarkInactiveForSession not supported on this platform")
}

func MarkInactiveAll(ctx context.Context, q InactiveQuerier) (int, error) {
	return 0, errors.New("timer: MarkInactiveAll not supported on this platform")
}
