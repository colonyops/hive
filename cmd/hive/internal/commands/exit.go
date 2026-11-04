package commands

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/domain/kv"
	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/pkg/iojson"
)

// ExitError ends the hive program with Code and prints Msg when it is not
// empty. It is not a cli.ExitCoder on purpose: urfave/cli calls os.Exit for
// those inside Run, which would end a test that drives a command.
type ExitError struct {
	Code int
	Msg  string
}

func (e ExitError) Error() string { return e.Msg }

// errAlreadyReported exits 1 after the command wrote its own error output.
var errAlreadyReported = ExitError{Code: 1}

const outputMetadataKey = "output"

// alwaysJSON marks a command that prints JSON without a --json flag, so its
// errors render as JSON too.
func alwaysJSON() map[string]any {
	return map[string]any{outputMetadataKey: "json"}
}

func printsJSON(c *cli.Command) bool {
	if c.Metadata[outputMetadataKey] == "json" {
		return true
	}
	return c.Bool("json")
}

// kindError gives an error a Kind without changing its text, for the
// commands whose messages already read well without a "kind:" prefix.
type kindError struct {
	kind ops.Kind
	err  error
}

func (e *kindError) Error() string { return e.err.Error() }
func (e *kindError) Unwrap() error { return e.err }

// invalidf is a usage or argument error: the caller asked for something the
// command cannot do, and rerunning the same call fails the same way.
func invalidf(format string, args ...any) error {
	return &kindError{kind: ops.KindInvalid, err: fmt.Errorf(format, args...)}
}

// notFoundf is a lookup that found nothing the command could act on.
func notFoundf(format string, args ...any) error {
	return &kindError{kind: ops.KindNotFound, err: fmt.Errorf(format, args...)}
}

// ReportedError is an error that left a command, tagged with the kind and
// output mode of the command that returned it.
type ReportedError struct {
	Err  error
	Kind ops.Kind
	JSON bool
}

func (e *ReportedError) Error() string { return e.Err.Error() }
func (e *ReportedError) Unwrap() error { return e.Err }

// ApplyErrorRules wraps the action of every command below root so that the
// error it returns carries its kind and whether the command prints JSON.
// urfave/cli does not tell the caller of Run which command failed, so the
// command has to say so itself.
func ApplyErrorRules(root *cli.Command) {
	for _, sub := range root.Commands {
		ApplyErrorRules(sub)
	}
	action := root.Action
	if action == nil {
		return
	}
	root.Action = func(ctx context.Context, c *cli.Command) error {
		err := action(ctx, c)
		if err == nil {
			return nil
		}
		if _, ok := errors.AsType[*ReportedError](err); ok {
			return err
		}
		return &ReportedError{Err: err, Kind: ErrorKind(err), JSON: printsJSON(c)}
	}
}

// ErrorKind classifies err. A *ops.Error and a kindError name their kind;
// the sentinel errors of the stores the commands call map to the kind a
// caller can act on; anything else is internal.
func ErrorKind(err error) ops.Kind {
	if err == nil {
		return ""
	}
	if opErr, ok := errors.AsType[*ops.Error](err); ok {
		return opErr.Kind
	}
	if kindErr, ok := errors.AsType[*kindError](err); ok {
		return kindErr.kind
	}
	switch {
	case errors.Is(err, session.ErrNotFound),
		errors.Is(err, hc.ErrNotFound),
		errors.Is(err, review.ErrSessionNotFound),
		errors.Is(err, messaging.ErrTopicNotFound),
		errors.Is(err, drive.ErrNotFound),
		errors.Is(err, kv.ErrNotFound):
		return ops.KindNotFound
	case errors.Is(err, session.ErrInvalidTag),
		errors.Is(err, wire.ErrRemoteRequired),
		errors.Is(err, hc.ErrInvalidItemType),
		errors.Is(err, hc.ErrInvalidStatus),
		errors.Is(err, drive.ErrInvalidSpace),
		errors.Is(err, drive.ErrInvalidPath),
		errors.Is(err, drive.ErrInvalidFrontmatter):
		return ops.KindInvalid
	case errors.Is(err, session.ErrDuplicateName),
		errors.Is(err, drive.ErrExists),
		errors.Is(err, drive.ErrNotEmpty):
		return ops.KindConflict
	}
	var unreachable *wire.UnreachableError
	var desktopUnreachable *desktopUnreachableError
	var desktopDown desktopNotRunningError
	if errors.As(err, &unreachable) || errors.As(err, &desktopUnreachable) || errors.As(err, &desktopDown) {
		return ops.KindUnavailable
	}
	return ops.KindInternal
}

// ExitCode is the process exit status for err.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := errors.AsType[ExitError](err); ok {
		return exitErr.Code
	}
	return ops.ExitCode(ErrorKind(err))
}

type errorLine struct {
	Kind    ops.Kind `json:"kind"`
	Message string   `json:"message"`
}

// ReportError prints err to w the way the failing command prints its output:
// one {"kind","message"} JSON line for a command in JSON mode, text
// otherwise. It returns the exit status. An ExitError with no message prints
// nothing, because the command already reported.
func ReportError(w io.Writer, err error) int {
	if err == nil {
		return 0
	}
	code := ExitCode(err)
	if _, ok := errors.AsType[ExitError](err); ok && err.Error() == "" {
		return code
	}

	const serversHint = "Check your servers with: hive servers ls"
	unreachable, ok := errors.AsType[*wire.UnreachableError](err)
	hint := ok && unreachable.Server != ""

	if reported, ok := errors.AsType[*ReportedError](err); ok && reported.JSON {
		message := err.Error()
		if opErr, ok := errors.AsType[*ops.Error](err); ok {
			message = opErr.Msg
		}
		if hint {
			message += ". " + serversHint
		}
		if writeErr := iojson.WriteLine(w, errorLine{Kind: ErrorKind(err), Message: message}); writeErr == nil {
			return code
		}
	}

	if msg := err.Error(); msg != "" {
		_, _ = fmt.Fprintln(w, msg)
	}
	if hint {
		_, _ = fmt.Fprintln(w, serversHint)
	}
	return code
}
