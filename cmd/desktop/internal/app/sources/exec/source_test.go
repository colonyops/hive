package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

// hostEnvironment is the resolver's stand-in: tests exercise decoding and
// failure semantics, not PATH resolution, which execenv owns and tests itself.
type hostEnvironment struct{}

func (hostEnvironment) Environ(context.Context) []string { return os.Environ() }

func newSource(t *testing.T, command string) *source {
	t.Helper()
	return &source{
		id:      "flow/node",
		topic:   "source:flow/node",
		command: command,
		timeout: 10 * time.Second,
		environ: hostEnvironment{},
	}
}

// produce collects what one run emitted, which for a failed run must be
// nothing at all: a half-emitted snapshot would tell the producer the items the
// command never reached are gone.
func produce(t *testing.T, s *source) ([]models.Msg, error) {
	t.Helper()
	var got []models.Msg
	err := s.Produce(t.Context(), func(msg models.Msg) error {
		got = append(got, msg)
		return nil
	})
	return got, err
}

func TestProduce_EmitsOneMessagePerItem(t *testing.T) {
	s := newSource(t, `echo '[{"id":"a","title":"Alpha"},{"id":"b","title":"Beta"}]'`)

	got, err := produce(t, s)
	require.NoError(t, err)

	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].Key)
	assert.Equal(t, "source:flow/node", got[0].Topic)
	assert.Equal(t, SourceKind, got[0].SourceKind)
	assert.JSONEq(t, `{"id":"a","title":"Alpha"}`, string(got[0].Payload))
	assert.Equal(t, "b", got[1].Key)
}

func TestProduce_NumericIDIsTheKey(t *testing.T) {
	got, err := produce(t, newSource(t, `echo '[{"id":42}]'`))
	require.NoError(t, err)

	require.Len(t, got, 1)
	assert.Equal(t, "42", got[0].Key)
}

// An empty array is the one legitimate way to say "this source owns nothing
// right now". It must succeed, because succeeding is what archives the items
// that are genuinely gone.
func TestProduce_EmptyArrayIsASuccessfulEmptySnapshot(t *testing.T) {
	got, err := produce(t, newSource(t, `echo '[]'`))

	require.NoError(t, err)
	assert.Empty(t, got)
}

// The central failure rule: everything below must be an error, because the
// producer treats a successful Produce as authoritative and would archive every
// item this source owns.
func TestProduce_BrokenRunsFailRatherThanReadAsEmpty(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct{ command, wants string }{
		"non-zero exit":       {`echo '[]'; echo "boom" >&2; exit 3`, "exit status 3"},
		"no output":           {`true`, "printed nothing"},
		"null":                {`echo null`, "null"},
		"bare object":         {`echo '{"id":"a"}'`, "a single object"},
		"ndjson":              {`printf '{"id":"a"}\n{"id":"b"}\n'`, "a single object"},
		"invalid json":        {`echo '[{"id":'`, "not valid JSON"},
		"trailing content":    {`echo '[] junk'`, "not valid JSON"},
		"item without an id":  {`echo '[{"title":"no id"}]'`, `no top-level "id"`},
		"blank id":            {`echo '[{"id":"  "}]'`, `no top-level "id"`},
		"duplicate ids":       {`echo '[{"id":"a"},{"id":"a"}]'`, `share the id "a"`},
		"non-object item":     {`echo '["a"]'`, "not an object"},
		"stderr but exits ok": {`echo '[{"id":"a"}]' >&2`, "printed nothing"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := produce(t, newSource(t, tc.command))

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wants)
			assert.Contains(t, err.Error(), `exec source "flow/node"`)
			assert.Empty(t, got, "a failed run must emit nothing")
		})
	}
}

func TestProduce_FailureCarriesStderr(t *testing.T) {
	_, err := produce(t, newSource(t, `echo "gcx: not logged in" >&2; exit 1`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "gcx: not logged in")
}

func TestProduce_TimeoutFails(t *testing.T) {
	s := newSource(t, `sleep 30`)
	s.timeout = 50 * time.Millisecond

	_, err := produce(t, s)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not finish within 50ms")
}

// A cancelled context is the app shutting down, not the command misbehaving.
func TestProduce_CancellationReportsTheCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := newSource(t, `echo '[]'`).Produce(ctx, func(models.Msg) error { return nil })

	require.ErrorIs(t, err, context.Canceled)
}

// Truncated output that happened to parse would be a short snapshot, which
// archives everything it dropped — so overflowing the cap fails the run.
func TestProduce_OversizedStdoutFails(t *testing.T) {
	_, err := produce(t, newSource(t, `head -c 1200000 /dev/zero | tr '\0' 'x'`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than 1048576 bytes")
}

func TestProduce_RunsInTheConfiguredDirectory(t *testing.T) {
	dir := t.TempDir()
	s := newSource(t, `printf '[{"id":"%s"}]' "$(pwd)"`)
	s.cwd = dir

	got, err := produce(t, s)
	require.NoError(t, err)

	require.Len(t, got, 1)
	// macOS resolves TempDir through /private, so compare what the shell saw.
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, resolved, got[0].Key)
}

func TestProduce_AddsTheConfiguredEnvironment(t *testing.T) {
	s := newSource(t, `printf '[{"id":"%s"}]' "$HIVE_TEST_TOKEN"`)
	s.env = map[string]string{"HIVE_TEST_TOKEN": "sentinel"}

	got, err := produce(t, s)
	require.NoError(t, err)

	require.Len(t, got, 1)
	assert.Equal(t, "sentinel", got[0].Key)
}

func TestProduce_InheritsTheResolvedEnvironment(t *testing.T) {
	t.Setenv("HIVE_TEST_INHERITED", "yes")
	s := newSource(t, `printf '[{"id":"%s"}]' "$HIVE_TEST_INHERITED"`)

	got, err := produce(t, s)
	require.NoError(t, err)

	require.Len(t, got, 1)
	assert.Equal(t, "yes", got[0].Key)
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"/tmp/x", "/tmp/x"},
		{"~", home},
		{"~/src", filepath.Join(home, "src")},
		{"~notauser/src", "~notauser/src"},
	} {
		got, err := expandHome(tc.in)
		require.NoErrorf(t, err, "expandHome(%q)", tc.in)
		assert.Equalf(t, tc.want, got, "expandHome(%q)", tc.in)
	}
}
