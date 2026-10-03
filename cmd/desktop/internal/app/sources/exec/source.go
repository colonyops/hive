package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	osexec "os/exec"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/pathutil"
)

// maxStdout caps what one run may print, matching the webhook listener's
// per-delivery cap so the same payload is the same size limit whichever way it
// arrives. Exceeding it fails the run rather than truncating: truncated JSON
// that happened to parse would be a *short snapshot*, which archives everything
// it dropped.
const maxStdout = 1 << 20

// maxStderrExcerpt is how much of a failed run's stderr rides along in the
// error. Enough to carry the message a CLI prints before exiting; not enough to
// fill the activity log with a stack trace.
const maxStderrExcerpt = 2048

// killGrace bounds how long a timed-out run blocks after its context is
// cancelled. CommandContext SIGKILLs `sh`, but a descendant that inherited the
// stdout pipe keeps it open and Wait would block on the copy until that
// grandchild exits on its own. Same mechanism as dispatch's shellKillGrace.
const killGrace = 2 * time.Second

// source runs one node's command and turns its stdout into the node's current
// items. It emits one message per item — not one per run carrying the whole
// output — because per-item identity is what makes absence, lifecycle and
// notifications work; a function node splitting one message afterwards gets
// none of those (ADR function-node-per-entity-feed-items, ADR a-command-is-a-source).
type source struct {
	id      string
	topic   string
	command string
	cwd     string
	env     map[string]string
	timeout time.Duration
	environ Environment
}

var _ connector.PullSource = (*source)(nil)

// Produce runs the command and emits its items.
//
// Every failure returns before the first emit, and that ordering is the whole
// contract: a successful Produce is an authoritative snapshot, so a run that
// half-emitted and then failed would tell the producer that the items it never
// reached are gone. A non-zero exit, a timeout, and output that is not a JSON
// array of identified items are all errors — never an empty snapshot.
func (s *source) Produce(ctx context.Context, emit func(models.Msg) error) error {
	stdout, err := s.run(ctx)
	if err != nil {
		return fmt.Errorf("exec source %q: %w", s.id, err)
	}
	items, err := decodeSnapshot(stdout)
	if err != nil {
		return fmt.Errorf("exec source %q: %w", s.id, err)
	}
	for _, item := range items {
		if err := emit(models.Msg{Key: item.key, Topic: s.topic, SourceKind: SourceKind, Payload: item.payload}); err != nil {
			return err
		}
	}
	return nil
}

// run executes the command and returns its stdout.
func (s *source) run(ctx context.Context) ([]byte, error) {
	dir, err := pathutil.ExpandHomeE(s.cwd)
	if err != nil {
		return nil, fmt.Errorf("resolving %q: %w", s.cwd, err)
	}

	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	cmd := osexec.CommandContext(runCtx, "sh", "-c", s.command)
	cmd.WaitDelay = killGrace
	cmd.Dir = dir
	cmd.Env = s.commandEnv(runCtx)
	stdout := &executil.HeadWriter{Max: maxStdout}
	stderr := &executil.HeadWriter{Max: maxStderrExcerpt}
	cmd.Stdout, cmd.Stderr = stdout, stderr

	runErr := cmd.Run()
	switch {
	case stdout.Truncated():
		return nil, fmt.Errorf("the command printed more than %d bytes to stdout", maxStdout)
	case runErr == nil:
		return stdout.Bytes(), nil
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		return nil, fmt.Errorf("the command did not finish within %s%s", s.timeout, stderrSuffix(stderr))
	default:
		return nil, fmt.Errorf("the command failed: %w%s", runErr, stderrSuffix(stderr))
	}
}

// commandEnv is the resolved environment plus the node's own variables. The
// node's win: they are appended, and a later assignment shadows an earlier one.
func (s *source) commandEnv(ctx context.Context) []string {
	env := s.environ.Environ(ctx)
	for name, value := range s.env {
		env = append(env, name+"="+value)
	}
	return env
}

// item is one decoded snapshot entry: the key it is tracked under and the
// object the command printed, forwarded verbatim.
type item struct {
	key     string
	payload json.RawMessage
}

// decodeSnapshot parses stdout into the run's items. It is strict on purpose —
// every rejection here is a case that would otherwise ingest as a *smaller*
// snapshot than the command meant, silently archiving whatever it dropped.
func decodeSnapshot(stdout []byte) ([]item, error) {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return nil, errors.New("the command printed nothing on stdout; an empty snapshot is `[]`")
	}
	// Checked before decoding so `null` — which unmarshals into a slice
	// without error, as nothing — cannot read as an empty snapshot.
	if trimmed[0] != '[' {
		return nil, fmt.Errorf("stdout must be a JSON array of items, not %s", describeJSON(trimmed))
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil, fmt.Errorf("stdout is not valid JSON: %w", err)
	}

	items := make([]item, 0, len(raw))
	seen := make(map[string]int, len(raw))
	for i, entry := range raw {
		entry = bytes.TrimSpace(entry)
		if len(entry) == 0 || entry[0] != '{' {
			return nil, fmt.Errorf("item %d is %s, not an object", i, describeJSON(entry))
		}
		id, _, _ := models.CanonicalFields(entry)
		if id == "" {
			return nil, fmt.Errorf(`item %d has no top-level "id"; an item's id is its identity across runs`, i)
		}
		if first, dup := seen[id]; dup {
			return nil, fmt.Errorf("items %d and %d share the id %q", first, i, id)
		}
		seen[id] = i
		items = append(items, item{key: id, payload: entry})
	}
	return items, nil
}

// describeJSON names what a fragment is, for an error a user can act on.
func describeJSON(fragment []byte) string {
	if len(fragment) == 0 {
		return "empty"
	}
	switch fragment[0] {
	case '{':
		return "a single object"
	case '[':
		return "an array"
	case '"':
		return "a string"
	case 'n':
		return "null"
	case 't', 'f':
		return "a boolean"
	default:
		return "a number or unquoted text"
	}
}

func stderrSuffix(stderr *executil.HeadWriter) string {
	text := strings.TrimSpace(stderr.String())
	if text == "" {
		return ""
	}
	return ": " + text
}
