package js_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
)

type fakeKV struct {
	rows map[string]string
	ttls map[string]int64
}

func newFakeKV() *fakeKV {
	return &fakeKV{rows: map[string]string{}, ttls: map[string]int64{}}
}

func (f *fakeKV) Get(_ context.Context, key string) (string, bool, error) {
	value, ok := f.rows[key]
	return value, ok, nil
}

func (f *fakeKV) Set(_ context.Context, key, value string, ttlSeconds int64) error {
	f.rows[key] = value
	f.ttls[key] = ttlSeconds
	return nil
}

func (f *fakeKV) Has(_ context.Context, key string) (bool, error) {
	_, ok := f.rows[key]
	return ok, nil
}

func (f *fakeKV) Delete(_ context.Context, key string) error {
	delete(f.rows, key)
	return nil
}

func (f *fakeKV) Keys(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	for key := range f.rows {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func runWithKV(t *testing.T, inst runtime.ScriptInstance, m models.Msg, kv runtime.NodeKV) ([][]models.Msg, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	return inst.OnMessage(ctx, m, map[string]any{}, kv, nil)
}

func TestKV_RoundTrip(t *testing.T) {
	t.Parallel()
	kv := newFakeKV()
	inst := instance(t, `
		kv.set('obj', {a: 1})
		const out = {
			v: kv.get('obj'),
			has: kv.has('obj'),
			missing: kv.get('nope'),
			hasMissing: kv.has('nope'),
			keys: kv.keys(''),
		}
		kv.delete('obj')
		out.afterDelete = kv.has('obj')
		return {...msg, Payload: out}
	`, 1)

	produced, err := runWithKV(t, inst, msg(`{}`), kv)
	require.NoError(t, err)
	require.JSONEq(t, `{"v":{"a":1},"has":true,"hasMissing":false,"keys":["obj"],"afterDelete":false}`, string(produced[0][0].Payload))
	assert.Empty(t, kv.rows)
}

func TestKV_ValuesPersistAcrossMessagesThroughTheHandle(t *testing.T) {
	t.Parallel()
	kv := newFakeKV()
	inst := instance(t, `
		const n = (kv.get('n') ?? 0) + 1
		kv.set('n', n)
		return {...msg, Payload: {n}}
	`, 1)

	for want := 1; want <= 3; want++ {
		produced, err := runWithKV(t, inst, msg(`{}`), kv)
		require.NoError(t, err)
		require.Contains(t, string(produced[0][0].Payload), `"n":`+string(rune('0'+want)))
	}
}

func TestKV_SetRejectsNonSerializableValues(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]string{
		"undefined": `kv.set('k', undefined)`,
		"function":  `kv.set('k', () => 1)`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			kv := newFakeKV()
			_, err := runWithKV(t, instance(t, src, 1), msg(`{}`), kv)
			var scriptErr *runtime.ScriptError
			require.ErrorAs(t, err, &scriptErr)
			assert.Equal(t, runtime.ScriptErrorRuntime, scriptErr.Kind)
			assert.Contains(t, scriptErr.Message, "not JSON-serializable")
			assert.Empty(t, kv.rows, "nothing may be stored, especially not an empty string")
		})
	}
}

func TestKV_TTLContract(t *testing.T) {
	t.Parallel()

	for name, src := range map[string]string{
		"negative":   `kv.set('k', 1, {ttl: -1})`,
		"fractional": `kv.set('k', 1, {ttl: 1.5})`,
		"nan":        `kv.set('k', 1, {ttl: NaN})`,
		"infinity":   `kv.set('k', 1, {ttl: Infinity})`,
		"overflow":   `kv.set('k', 1, {ttl: 1e18})`,
		"string":     `kv.set('k', 1, {ttl: '60'})`,
	} {
		t.Run(name+" throws", func(t *testing.T) {
			t.Parallel()
			_, err := runWithKV(t, instance(t, src, 1), msg(`{}`), newFakeKV())
			var scriptErr *runtime.ScriptError
			require.ErrorAs(t, err, &scriptErr)
			assert.Contains(t, scriptErr.Message, "ttl")
		})
	}

	t.Run("omitted and zero mean no expiry; a positive ttl passes through", func(t *testing.T) {
		t.Parallel()
		kv := newFakeKV()
		_, err := runWithKV(t, instance(t, `kv.set('a', 1); kv.set('b', 1, {ttl: 0}); kv.set('c', 1, {ttl: 60})`, 1), msg(`{}`), kv)
		require.NoError(t, err)
		assert.Equal(t, int64(0), kv.ttls["a"])
		assert.Equal(t, int64(0), kv.ttls["b"])
		assert.Equal(t, int64(60), kv.ttls["c"])
	})
}

func TestKV_Caps(t *testing.T) {
	t.Parallel()

	t.Run("over-cap key throws", func(t *testing.T) {
		t.Parallel()
		kv := newFakeKV()
		_, err := runWithKV(t, instance(t, `kv.set('k'.repeat(513), 1)`, 1), msg(`{}`), kv)
		var scriptErr *runtime.ScriptError
		require.ErrorAs(t, err, &scriptErr)
		assert.Contains(t, scriptErr.Message, "key exceeds")
		assert.Empty(t, kv.rows)
	})

	t.Run("over-cap value throws", func(t *testing.T) {
		t.Parallel()
		kv := newFakeKV()
		_, err := runWithKV(t, instance(t, `kv.set('k', 'v'.repeat(5000))`, 1), msg(`{}`), kv)
		var scriptErr *runtime.ScriptError
		require.ErrorAs(t, err, &scriptErr)
		assert.Contains(t, scriptErr.Message, "value exceeds")
		assert.Empty(t, kv.rows)
	})
}

func TestKV_NonStringKeysThrow(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]string{
		"get":    `kv.get(1)`,
		"set":    `kv.set({}, 1)`,
		"keys":   `kv.keys(7)`,
		"delete": `kv.delete(null)`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := runWithKV(t, instance(t, src, 1), msg(`{}`), newFakeKV())
			var scriptErr *runtime.ScriptError
			require.ErrorAs(t, err, &scriptErr)
			assert.Contains(t, scriptErr.Message, "must be a string")
		})
	}
}

func TestKV_UnavailableWithoutAHandle(t *testing.T) {
	t.Parallel()
	_, err := runWithKV(t, instance(t, `kv.get('k')`, 1), msg(`{}`), nil)
	var scriptErr *runtime.ScriptError
	require.ErrorAs(t, err, &scriptErr)
	assert.Equal(t, runtime.ScriptErrorRuntime, scriptErr.Kind)
	assert.Contains(t, scriptErr.Message, "unavailable")
}

// A script can stash `kv` or a bound method in `state` on one message; both
// must resolve the invocation that is actually running, not the one they
// were captured in.
func TestKV_RetainedReferencesTargetTheLiveInvocation(t *testing.T) {
	t.Parallel()
	inst := instance(t, `
		if (!state.put) {
			state.kv = kv
			state.put = kv.set
			kv.set('captured-on', 'first')
			return null
		}
		state.put('via-bound-method', 1)
		state.kv.set('via-object', 2)
		return {...msg, Payload: {read: state.kv.get('captured-on') ?? null}}
	`, 1)

	first := newFakeKV()
	_, err := runWithKV(t, inst, msg(`{}`), first)
	require.NoError(t, err)
	require.Contains(t, first.rows, "captured-on")

	second := newFakeKV()
	produced, err := runWithKV(t, inst, msg(`{}`), second)
	require.NoError(t, err)

	assert.Contains(t, second.rows, "via-bound-method")
	assert.Contains(t, second.rows, "via-object")
	assert.NotContains(t, first.rows, "via-bound-method", "the retained reference must not reach the first message's handle")
	require.JSONEq(t, `{"read":null}`, string(produced[0][0].Payload), "reads go to the live handle, which never saw the first message's write")
}

func TestKV_ScriptsWithoutKVAreUnaffected(t *testing.T) {
	t.Parallel()
	produced, err := runWithKV(t, instance(t, `return msg`, 1), msg(`{"n":1}`), nil)
	require.NoError(t, err)
	require.JSONEq(t, `{"n":1}`, string(produced[0][0].Payload))
}
