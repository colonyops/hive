package js_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime/js"
)

func msg(payload string) models.Msg {
	return models.Msg{
		ID: "1", Key: "k", Topic: "source:f/src", Ts: 7,
		Payload: json.RawMessage(payload), SourceKind: "github", SourceScope: "acme/app",
	}
}

func instance(t *testing.T, src string, outputs int) runtime.ScriptInstance {
	t.Helper()
	inst, err := js.New(nil).New(src, outputs)
	require.NoError(t, err)
	t.Cleanup(inst.Close)
	return inst
}

func run(t *testing.T, inst runtime.ScriptInstance, m models.Msg) [][]models.Msg {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	produced, err := inst.OnMessage(ctx, m, map[string]any{}, nil, nil)
	require.NoError(t, err)
	return produced
}

// The overloaded return shapes are resolved by the node's declared output
// count, not by inspecting the value: Msg[] and a port-indexed array are both
// plain arrays, and guessing between them would route every message down one
// branch.
func TestReturnShapes(t *testing.T) {
	t.Parallel()

	t.Run("a single object goes out port 0", func(t *testing.T) {
		t.Parallel()
		produced := run(t, instance(t, "return msg", 1), msg(`{"n":1}`))
		require.Len(t, produced, 1)
		require.Len(t, produced[0], 1)
		require.JSONEq(t, `{"n":1}`, string(produced[0][0].Payload))
	})

	t.Run("an array on a one-output node is several messages on port 0", func(t *testing.T) {
		t.Parallel()
		produced := run(t, instance(t, "return [msg, msg]", 1), msg(`{"n":1}`))
		require.Len(t, produced, 1)
		require.Len(t, produced[0], 2)
	})

	t.Run("an array on a multi-output node is indexed by port", func(t *testing.T) {
		t.Parallel()
		produced := run(t, instance(t, "return [msg, null, [msg, msg]]", 3), msg(`{"n":1}`))
		require.Len(t, produced, 3)
		require.Len(t, produced[0], 1)
		require.Empty(t, produced[1])
		require.Len(t, produced[2], 2)
	})

	t.Run("null is a discard", func(t *testing.T) {
		t.Parallel()
		require.True(t, empty(run(t, instance(t, "return null", 1), msg(`{}`))))
	})

	t.Run("returning nothing is a discard", func(t *testing.T) {
		t.Parallel()
		require.True(t, empty(run(t, instance(t, "msg.Payload.x = 1", 1), msg(`{}`))))
	})

	// The browser silently dropped these because no wire exists on that port.
	// Surfacing them as messages keeps the engine's discard accounting honest
	// instead of losing them before it can count them.
	t.Run("ports beyond the declared count are still returned", func(t *testing.T) {
		t.Parallel()
		produced := run(t, instance(t, "return [null, null, msg]", 2), msg(`{}`))
		require.Len(t, produced, 3)
		require.Len(t, produced[2], 1)
	})
}

// This is the property that ruled Lua out. Every one of these values is
// distinguishable in JSON and would need a sentinel or a marker in a language
// whose tables cannot tell them apart.
func TestJSONFidelity(t *testing.T) {
	t.Parallel()

	produced := run(t, instance(t, "return msg", 1), msg(`{"nil":null,"arr":[],"obj":{},"z":1,"a":2}`))
	encoded := string(produced[0][0].Payload)

	require.JSONEq(t, `{"nil":null,"arr":[],"obj":{},"z":1,"a":2}`, encoded)
	require.Contains(t, encoded, `"nil":null`, "null survives as null rather than a missing key")
	require.Contains(t, encoded, `"arr":[]`, "an empty array stays an array")
	require.Contains(t, encoded, `"obj":{}`, "an empty object stays an object")
	require.Less(t, indexOf(encoded, `"z"`), indexOf(encoded, `"a"`), "key order is preserved, not sorted")
}

// Array.isArray is the tell: goja's wrapper for a Go slice answers false,
// which is why values cross the boundary through the VM's own JSON.parse
// instead of ToValue.
func TestPayloadIsARealJavaScriptValue(t *testing.T) {
	t.Parallel()

	produced := run(t, instance(t, "return {...msg, Payload: {arr: Array.isArray(msg.Payload.labels), keys: Object.keys(msg.Payload)}}", 1), msg(`{"labels":["a"]}`))
	require.JSONEq(t, `{"arr":true,"keys":["labels"]}`, string(produced[0][0].Payload))
}

// State is one object for the life of the instance, so lazy init works and a
// counter accumulates. This is what replaces the on_start hook.
func TestStateSurvivesAcrossMessages(t *testing.T) {
	t.Parallel()

	inst := instance(t, "state.n = (state.n ?? 0) + 1; msg.Payload.n = state.n; return msg", 1)
	require.JSONEq(t, `{"n":1}`, string(run(t, inst, msg(`{}`))[0][0].Payload))
	require.JSONEq(t, `{"n":2}`, string(run(t, inst, msg(`{}`))[0][0].Payload))
	require.JSONEq(t, `{"n":3}`, string(run(t, inst, msg(`{}`))[0][0].Payload))
}

func TestNodeConfigIsTheSecondArgument(t *testing.T) {
	t.Parallel()

	inst := instance(t, "return {...msg, Payload: {seen: node.label}}", 1)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	produced, err := inst.OnMessage(ctx, msg(`{}`), map[string]any{"label": "hello"}, nil, nil)
	require.NoError(t, err)
	require.JSONEq(t, `{"seen":"hello"}`, string(produced[0][0].Payload))
}

func TestCompileErrorsCarryTheAuthorsPosition(t *testing.T) {
	t.Parallel()

	_, err := js.New(nil).New("const a = 1\nreturn msg(", 1)

	var scriptErr *runtime.ScriptError
	require.ErrorAs(t, err, &scriptErr)
	require.Equal(t, runtime.ScriptErrorCompile, scriptErr.Kind)
	require.Equal(t, 2, scriptErr.Line, "line 2 of what the author wrote, not of the wrapper the engine compiles")
	require.NotEmpty(t, scriptErr.Message)
}

func TestCheckReportsSyntaxWithoutRunning(t *testing.T) {
	t.Parallel()

	rt := js.New(nil)
	require.NoError(t, rt.Check("return msg"))
	require.Error(t, rt.Check("return msg("))
	require.NoError(t, rt.Check("throw new Error('only at run time')"))
}

func TestThrownValuesAreRuntimeErrors(t *testing.T) {
	t.Parallel()

	inst := instance(t, "\nthrow new Error('nope')", 1)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err := inst.OnMessage(ctx, msg(`{}`), map[string]any{}, nil, nil)

	var scriptErr *runtime.ScriptError
	require.ErrorAs(t, err, &scriptErr)
	require.Equal(t, runtime.ScriptErrorRuntime, scriptErr.Kind)
	require.Contains(t, scriptErr.Message, "nope")
	require.Equal(t, 2, scriptErr.Line)
}

// Line is corrected against the wrapper, but goja writes its own positions into
// the rendered message and stack. Left alone the two disagree — line 2
// structurally, "on_message:3" in the prose — and whichever one the reader
// trusts, the other sends them to the wrong line.
func TestRenderedPositionsAgreeWithTheStructuredLine(t *testing.T) {
	t.Parallel()

	inst := instance(t, "\nthrow new Error('nope')", 1)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, err := inst.OnMessage(ctx, msg(`{}`), map[string]any{}, nil, nil)

	var scriptErr *runtime.ScriptError
	require.ErrorAs(t, err, &scriptErr)
	require.Equal(t, 2, scriptErr.Line)
	assert.Contains(t, scriptErr.Message, "on_message:2:")
	assert.NotContains(t, scriptErr.Message, "on_message:3:")
	assert.Contains(t, scriptErr.Stack, "on_message:2:")
	assert.NotContains(t, scriptErr.Stack, "on_message:3:")
}

// A runaway loop is stopped by interrupting the VM, and the caller gets its
// goroutine back. This is what replaces the browser's worker.terminate().
func TestARunawayLoopIsInterrupted(t *testing.T) {
	t.Parallel()

	inst := instance(t, "while (true) {}", 1)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := inst.OnMessage(ctx, msg(`{}`), map[string]any{}, nil, nil)
	elapsed := time.Since(started)

	var scriptErr *runtime.ScriptError
	require.ErrorAs(t, err, &scriptErr)
	require.Equal(t, runtime.ScriptErrorTimeout, scriptErr.Kind)
	require.Less(t, elapsed, 3*time.Second, "the caller must get control back rather than waiting on the script")
}

// The pool is the bound on abandoned work. Once every slot is held, a further
// evaluation fails immediately instead of queueing behind something nothing
// can stop.
func TestAnExhaustedPoolRefusesRatherThanQueues(t *testing.T) {
	t.Parallel()

	pool := runtime.NewScriptPool(1)
	require.True(t, pool.Acquire(), "take the only slot")

	inst, err := js.New(pool).New("return msg", 1)
	require.NoError(t, err)
	t.Cleanup(inst.Close)

	started := time.Now()
	_, err = inst.OnMessage(t.Context(), msg(`{}`), map[string]any{}, nil, nil)
	require.Less(t, time.Since(started), time.Second, "refusal is immediate")

	var scriptErr *runtime.ScriptError
	require.ErrorAs(t, err, &scriptErr)
	require.Equal(t, runtime.ScriptErrorUnavailable, scriptErr.Kind)

	pool.Release()
	_, err = inst.OnMessage(t.Context(), msg(`{}`), map[string]any{}, nil, nil)
	require.NoError(t, err, "the slot is usable again once released")
}

func TestPoolSlotsAreReturnedAfterEveryEvaluation(t *testing.T) {
	t.Parallel()

	pool := runtime.NewScriptPool(2)
	inst, err := js.New(pool).New("return msg", 1)
	require.NoError(t, err)
	t.Cleanup(inst.Close)

	for range 10 {
		_, err := inst.OnMessage(t.Context(), msg(`{}`), map[string]any{}, nil, nil)
		require.NoError(t, err)
	}
	require.Equal(t, 2, pool.Available())
}

func TestReturningSomethingThatIsNotAMessage(t *testing.T) {
	t.Parallel()

	inst := instance(t, "return 42", 1)
	_, err := inst.OnMessage(t.Context(), msg(`{}`), map[string]any{}, nil, nil)

	var scriptErr *runtime.ScriptError
	require.ErrorAs(t, err, &scriptErr)
	require.Equal(t, runtime.ScriptErrorResult, scriptErr.Kind)
}

func TestScriptErrorMessageIncludesPositionWhenKnown(t *testing.T) {
	t.Parallel()

	withPosition := &runtime.ScriptError{Kind: runtime.ScriptErrorCompile, Message: "bad", Line: 3, Column: 5}
	require.Equal(t, "compile:3:5: bad", withPosition.Error())

	without := &runtime.ScriptError{Kind: runtime.ScriptErrorTimeout, Message: "slow"}
	require.Equal(t, "timeout: slow", without.Error())
}

func empty(ports [][]models.Msg) bool {
	for _, port := range ports {
		if len(port) > 0 {
			return false
		}
	}
	return true
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
