package js_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
)

func collectConsole(t *testing.T, src string) []runtime.ConsoleLine {
	t.Helper()
	var lines []runtime.ConsoleLine
	inst := instance(t, src, 1)
	_, err := inst.OnMessage(t.Context(), msg(`{"n":1}`), map[string]any{}, nil,
		func(level, text string) { lines = append(lines, runtime.ConsoleLine{Level: level, Text: text}) })
	require.NoError(t, err)
	return lines
}

// Strings render verbatim and everything else as JSON, which is Node's shape
// minus format specifiers — an author debugging a payload wants to see the
// object, not "[object Object]".
func TestConsoleFormatsArguments(t *testing.T) {
	t.Parallel()

	lines := collectConsole(t, `
		console.log("plain");
		console.log("payload", msg.Payload, [1, null], null);
		console.error(new Error("boom").message);
		console.debug(undefined);
		return null;
	`)

	assert.Equal(t, []runtime.ConsoleLine{
		{Level: "log", Text: "plain"},
		{Level: "log", Text: `payload {"n":1} [1,null] null`},
		{Level: "error", Text: "boom"},
		{Level: "debug", Text: "undefined"},
	}, lines)
}

// A live run supplies no sink. console must still resolve — a script that logs
// is not a script that fails — and the calls must go nowhere.
func TestConsoleWithoutASinkIsANoOp(t *testing.T) {
	t.Parallel()

	inst := instance(t, `console.log("dropped"); return msg;`, 1)
	produced, err := inst.OnMessage(t.Context(), msg(`{}`), map[string]any{}, nil, nil)
	require.NoError(t, err)
	require.Len(t, produced, 1)
	require.Len(t, produced[0], 1)
}

// A value JSON.stringify cannot represent must not turn a debug log into a
// failed message.
func TestConsoleSurvivesAnUnserializableArgument(t *testing.T) {
	t.Parallel()

	lines := collectConsole(t, `
		const cyclic = {}; cyclic.self = cyclic;
		console.log(cyclic);
		console.log(function named() {});
		return null;
	`)

	require.Len(t, lines, 2)
	assert.NotEmpty(t, lines[0].Text)
	assert.NotEmpty(t, lines[1].Text)
}
