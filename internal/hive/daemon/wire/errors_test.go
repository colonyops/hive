package wire_test

import (
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
)

func roundTrip(err error) error {
	return wire.FromConnectError(wire.ToConnectError(err))
}

func TestSentinelsSurviveTheWire(t *testing.T) {
	for _, tc := range []struct {
		err      error
		sentinel error
		code     connect.Code
	}{
		{fmt.Errorf("get session: %w", session.ErrNotFound), session.ErrNotFound, connect.CodeNotFound},
		{fmt.Errorf("rename: %w", session.ErrDuplicateName), session.ErrDuplicateName, connect.CodeAlreadyExists},
		{wire.ErrRemoteRequired, wire.ErrRemoteRequired, connect.CodeInvalidArgument},
		{fmt.Errorf("%w: it is empty", session.ErrInvalidTag), session.ErrInvalidTag, connect.CodeInvalidArgument},
		{fmt.Errorf("session is recycled: %w", session.ErrNotActive), session.ErrNotActive, connect.CodeFailedPrecondition},
		{errors.New("disk full"), nil, connect.CodeInternal},
	} {
		got := roundTrip(tc.err)
		assert.Equal(t, tc.err.Error(), got.Error(), "the message reads as it did in-process")
		assert.Equal(t, tc.code, connect.CodeOf(got))
		if tc.sentinel != nil {
			assert.ErrorIs(t, got, tc.sentinel)
		}
	}
}

func TestInvalidArgumentSentinelsStayApart(t *testing.T) {
	require.NotErrorIs(t, roundTrip(fmt.Errorf("%w: it is empty", session.ErrInvalidTag)), wire.ErrRemoteRequired)
	require.NotErrorIs(t, roundTrip(wire.ErrRemoteRequired), session.ErrInvalidTag)
}

func TestLaunchErrorKeepsEveryFieldAndItsCause(t *testing.T) {
	cause := &sessionsvc.CreateSessionError{Operation: "clone repository", Destination: "/r/x", CloneStrategy: "full", Err: errors.New("exit 128")}
	original := &sessionsvc.LaunchError{
		Name: "x", Remote: "git@h:o/r", Destination: "/r/x", CloneStrategy: "full",
		LeftoverCheckout: true, Step: "clone repository", Output: "fatal: nope", Err: cause,
	}

	got := roundTrip(fmt.Errorf("create: %w", original))

	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(got))
	require.NotErrorIs(t, got, session.ErrNotActive, "a failed launch is not an inactive session")
	var launch *sessionsvc.LaunchError
	require.ErrorAs(t, got, &launch)
	assert.Equal(t, original.Name, launch.Name)
	assert.Equal(t, original.Remote, launch.Remote)
	assert.Equal(t, original.Destination, launch.Destination)
	assert.Equal(t, original.CloneStrategy, launch.CloneStrategy)
	assert.True(t, launch.LeftoverCheckout)
	assert.Equal(t, original.Step, launch.Step)
	assert.Equal(t, original.Output, launch.Output)

	var create *sessionsvc.CreateSessionError
	require.ErrorAs(t, got, &create)
	assert.Equal(t, "/r/x", create.Destination)
	assert.Equal(t, "clone repository", create.Operation)
	assert.Equal(t, "exit 128", create.Err.Error())
}
