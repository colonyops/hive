package wailsui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

func decodeWire(t *testing.T, raw []byte) wireError {
	t.Helper()
	var got wireError
	require.NoError(t, json.Unmarshal(raw, &got))
	return got
}

func TestMarshalError_CarriesTheCoreKind(t *testing.T) {
	t.Parallel()

	for _, kind := range []app.Kind{
		app.KindInternal, app.KindInvalid, app.KindNotFound,
		app.KindConflict, app.KindUnauthenticated, app.KindUnavailable,
	} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()

			got := decodeWire(t, MarshalError(app.Errorf(kind, "something happened")))
			assert.Equal(t, kind, got.Kind)
			assert.Equal(t, "something happened", got.Message)
		})
	}
}

func TestMarshalError_SeesThroughAWrappedChain(t *testing.T) {
	t.Parallel()

	// A service returning a typed error that a caller wraps again is the
	// normal shape; the Kind has to survive it, because the frontend has
	// nothing else to switch on.
	classified := app.Wrap(errors.New("no such row"), app.KindNotFound, "reading action run 7")
	got := decodeWire(t, MarshalError(fmt.Errorf("loading actions: %w", classified)))
	assert.Equal(t, app.KindNotFound, got.Kind)
	assert.Contains(t, got.Message, "no such row", "the chain still reaches the log")
}

func TestMarshalError_UnclassifiedIsInternal(t *testing.T) {
	t.Parallel()

	got := decodeWire(t, MarshalError(errors.New("something in a library broke")))
	assert.Equal(t, app.KindInternal, got.Kind)
	assert.Equal(t, "something in a library broke", got.Message, "a bare error still gets a non-empty message")
}

func TestMarshalError_NilIsNil(t *testing.T) {
	t.Parallel()

	assert.Nil(t, MarshalError(nil))
}

// TestAppErrorKindUnionMatchesTheCoreKinds is the bijection between the Go
// vocabulary and the TypeScript union that mirrors it. Nothing else can catch
// the drift: a Kind added in Go and missing from the union is narrowed to null
// by appErrorKind, so the frontend silently stops handling a case it was
// supposed to. Same pattern as the prompts registry↔docs test.
func TestAppErrorKindUnionMatchesTheCoreKinds(t *testing.T) {
	t.Parallel()

	const path = "../../../frontend/src/lib/appError.ts"
	source, err := os.ReadFile(path)
	require.NoError(t, err, "the frontend mirror of Kind must exist at %s", path)

	// The runtime list is what appErrorKind actually narrows against; the
	// exported type union is what callers switch on. Both have to agree with
	// Go, so both are checked.
	runtime := regexp.MustCompile(`(?s)const KINDS: readonly AppErrorKind\[\] = \[(.*?)\]`).FindSubmatch(source)
	require.NotNil(t, runtime, "could not find the KINDS list in %s", path)
	union := regexp.MustCompile(`(?s)export type AppErrorKind =(.*?)\n\n`).FindSubmatch(source)
	require.NotNil(t, union, "could not find the AppErrorKind union in %s", path)

	quoted := regexp.MustCompile(`'([a-z_]+)'`)
	var want []string
	for _, kind := range app.AllKinds() {
		want = append(want, string(kind))
	}

	for name, block := range map[string][]byte{"KINDS": runtime[1], "AppErrorKind": union[1]} {
		var got []string
		for _, match := range quoted.FindAllSubmatch(block, -1) {
			got = append(got, string(match[1]))
		}
		assert.ElementsMatch(t, want, got, "%s has drifted from app.AllKinds()", name)
	}
}
