package wire

import (
	"errors"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"github.com/colonyops/hive/internal/domain/session"
	daemonv1 "github.com/colonyops/hive/internal/hive/daemon/gen/hive/daemon/v1"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
)

// ErrRemoteRequired rejects a create with no remote. The daemon cannot detect
// one: its working directory is not the caller's.
var ErrRemoteRequired = errors.New("remote is required: the caller resolves it before calling the daemon")

// ToConnectError maps a session API error to a Connect error. A LaunchError
// or CreateSessionError anywhere in the chain travels as a detail, whatever
// the code, so the client can rebuild it for errors.As. It returns error, not
// *connect.Error, so a handler can return it for a nil err without sending a
// typed nil that Connect reports as a failure.
func ToConnectError(err error) error {
	if err == nil {
		return nil
	}
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return connectErr
	}

	var launchErr *sessionsvc.LaunchError
	hasLaunch := errors.As(err, &launchErr)
	var createErr *sessionsvc.CreateSessionError
	hasCreate := errors.As(err, &createErr)

	code := connect.CodeInternal
	switch {
	case errors.Is(err, session.ErrNotFound):
		code = connect.CodeNotFound
	case errors.Is(err, session.ErrDuplicateName):
		code = connect.CodeAlreadyExists
	case errors.Is(err, ErrRemoteRequired), errors.Is(err, session.ErrInvalidTag):
		code = connect.CodeInvalidArgument
	case hasLaunch || hasCreate:
		code = connect.CodeFailedPrecondition
	}

	out := connect.NewError(code, err)
	if errors.Is(err, session.ErrInvalidTag) {
		addDetail(out, &daemonv1.InvalidTagDetail{})
	}
	if hasLaunch {
		addDetail(out, &daemonv1.LaunchErrorDetail{
			Name:             launchErr.Name,
			Remote:           launchErr.Remote,
			Destination:      launchErr.Destination,
			CloneStrategy:    launchErr.CloneStrategy,
			LeftoverCheckout: launchErr.LeftoverCheckout,
			Step:             launchErr.Step,
			Output:           launchErr.Output,
			Err:              errText(launchErr.Err),
		})
	}
	if hasCreate {
		addDetail(out, &daemonv1.CreateSessionErrorDetail{
			Operation:     createErr.Operation,
			Destination:   createErr.Destination,
			CloneStrategy: createErr.CloneStrategy,
			Err:           errText(createErr.Err),
		})
	}
	return out
}

func addDetail(err *connect.Error, msg proto.Message) {
	detail, detailErr := connect.NewErrorDetail(msg)
	if detailErr == nil {
		err.AddDetail(detail)
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// FromConnectError rebuilds the Go error a client expects from a Connect
// error. errors.Is matches session.ErrNotFound, session.ErrDuplicateName,
// session.ErrInvalidTag, and ErrRemoteRequired by code and detail;
// errors.As finds a rebuilt LaunchError or CreateSessionError;
// connect.CodeOf still reads the code.
func FromConnectError(err error) error {
	var unreachable *UnreachableError
	if errors.As(err, &unreachable) {
		return err
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		return err
	}

	var createDetail *daemonv1.CreateSessionErrorDetail
	var launchDetail *daemonv1.LaunchErrorDetail
	invalidTag := false
	for _, detail := range connectErr.Details() {
		value, valueErr := detail.Value()
		if valueErr != nil {
			continue
		}
		switch d := value.(type) {
		case *daemonv1.CreateSessionErrorDetail:
			createDetail = d
		case *daemonv1.LaunchErrorDetail:
			launchDetail = d
		case *daemonv1.InvalidTagDetail:
			invalidTag = true
		}
	}
	sentinel := sentinelFor(connectErr.Code(), invalidTag)
	var inner error = &remoteError{conn: connectErr, sentinel: sentinel}
	switch {
	case createDetail != nil:
		inner = &sessionsvc.CreateSessionError{
			Operation:     createDetail.GetOperation(),
			Destination:   createDetail.GetDestination(),
			CloneStrategy: createDetail.GetCloneStrategy(),
			Err:           &remoteError{conn: connectErr, sentinel: sentinel, text: createDetail.GetErr()},
		}
	case launchDetail != nil:
		inner = &remoteError{conn: connectErr, sentinel: sentinel, text: launchDetail.GetErr()}
	}
	if launchDetail == nil {
		return inner
	}
	return &sessionsvc.LaunchError{
		Name:             launchDetail.GetName(),
		Remote:           launchDetail.GetRemote(),
		Destination:      launchDetail.GetDestination(),
		CloneStrategy:    launchDetail.GetCloneStrategy(),
		LeftoverCheckout: launchDetail.GetLeftoverCheckout(),
		Step:             launchDetail.GetStep(),
		Output:           launchDetail.GetOutput(),
		Err:              inner,
	}
}

func sentinelFor(code connect.Code, invalidTag bool) error {
	switch code {
	case connect.CodeNotFound:
		return session.ErrNotFound
	case connect.CodeAlreadyExists:
		return session.ErrDuplicateName
	case connect.CodeInvalidArgument:
		if invalidTag {
			return session.ErrInvalidTag
		}
		return ErrRemoteRequired
	default:
		return nil
	}
}

// remoteError reads as the daemon's message rather than Connect's
// "code: message" form, so CLI output does not change with the flag.
type remoteError struct {
	conn     *connect.Error
	sentinel error
	text     string
}

func (e *remoteError) Error() string {
	if e.text != "" {
		return e.text
	}
	return e.conn.Message()
}

func (e *remoteError) Unwrap() []error {
	if e.sentinel == nil {
		return []error{e.conn}
	}
	return []error{e.sentinel, e.conn}
}

// UnreachableError is a call that the daemon did not answer, as opposed to an
// error the daemon returned.
type UnreachableError struct {
	// Server is the server the call was for. Empty means this machine's daemon.
	Server  string
	message string
	err     error
}

// NewUnreachableError wraps err, the transport's failure, so that it prints as
// message.
func NewUnreachableError(server, message string, err error) *UnreachableError {
	return &UnreachableError{Server: server, message: message, err: err}
}

func (e *UnreachableError) Error() string { return e.message }
func (e *UnreachableError) Unwrap() error { return e.err }
