package js

import (
	"fmt"
	"math"

	"github.com/dop251/goja"

	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
)

// buildKVObject builds the stable `kv` façade once per instance. Each method
// resolves i.curKV/i.curCtx at call time, so a reference retained across
// messages always targets the live invocation. Contract violations and
// NodeKV failures throw into the script; an uncaught throw surfaces as an
// ordinary ScriptErrorRuntime and drops the message with its staging.
func (i *instance) buildKVObject() *goja.Object {
	obj := i.vm.NewObject()
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}

	must(obj.Set("get", func(call goja.FunctionCall) goja.Value {
		key := i.kvKey(call, "kv.get")
		value, found, err := i.kv("kv.get").Get(i.curCtx, key)
		if err != nil {
			panic(i.vm.NewGoError(err))
		}
		if !found {
			return goja.Undefined()
		}
		parsed, err := i.json.parse(goja.Undefined(), i.vm.ToValue(value))
		if err != nil {
			panic(i.vm.NewGoError(fmt.Errorf("kv.get: stored value is not valid JSON: %w", err)))
		}
		return parsed
	}))

	must(obj.Set("set", func(call goja.FunctionCall) goja.Value {
		key := i.kvKey(call, "kv.set")
		if len(key) > maxKVKeyBytes {
			panic(i.vm.NewTypeError("kv.set: key exceeds %d bytes", maxKVKeyBytes))
		}
		encoded, err := i.json.stringify(goja.Undefined(), call.Argument(1))
		if err != nil {
			panic(i.vm.NewGoError(err))
		}
		if encoded == nil || goja.IsUndefined(encoded) {
			panic(i.vm.NewTypeError("kv.set: value is not JSON-serializable"))
		}
		text := encoded.String()
		if len(text) > maxKVValueBytes {
			panic(i.vm.NewTypeError("kv.set: value exceeds %d bytes", maxKVValueBytes))
		}
		ttl := i.kvTTL(call.Argument(2))
		if err := i.kv("kv.set").Set(i.curCtx, key, text, ttl); err != nil {
			panic(i.vm.NewGoError(err))
		}
		return goja.Undefined()
	}))

	must(obj.Set("has", func(call goja.FunctionCall) goja.Value {
		key := i.kvKey(call, "kv.has")
		found, err := i.kv("kv.has").Has(i.curCtx, key)
		if err != nil {
			panic(i.vm.NewGoError(err))
		}
		return i.vm.ToValue(found)
	}))

	must(obj.Set("delete", func(call goja.FunctionCall) goja.Value {
		key := i.kvKey(call, "kv.delete")
		if err := i.kv("kv.delete").Delete(i.curCtx, key); err != nil {
			panic(i.vm.NewGoError(err))
		}
		return goja.Undefined()
	}))

	must(obj.Set("keys", func(call goja.FunctionCall) goja.Value {
		prefix := ""
		if arg := call.Argument(0); !goja.IsUndefined(arg) && !goja.IsNull(arg) {
			s, ok := arg.Export().(string)
			if !ok {
				panic(i.vm.NewTypeError("kv.keys: prefix must be a string"))
			}
			prefix = s
		}
		keys, err := i.kv("kv.keys").Keys(i.curCtx, prefix)
		if err != nil {
			panic(i.vm.NewGoError(err))
		}
		if keys == nil {
			keys = []string{}
		}
		value, jsErr := i.toJS(keys)
		if jsErr != nil {
			panic(i.vm.NewGoError(jsErr))
		}
		return value
	}))

	return obj
}

// kv resolves the live invocation's NodeKV, throwing when there is none
// (preview / dry-run, or a runner built without a KV port).
func (i *instance) kv(method string) runtime.NodeKV {
	if i.curKV == nil {
		panic(i.vm.NewTypeError("%s: kv is unavailable here", method))
	}
	return i.curKV
}

func (i *instance) kvKey(call goja.FunctionCall, method string) string {
	key, ok := call.Argument(0).Export().(string)
	if !ok {
		panic(i.vm.NewTypeError("%s: key must be a string", method))
	}
	return key
}

// kvTTL reads the optional {ttl} option: omitted, null or 0 means no expiry;
// anything else must be a finite, whole, non-negative number of seconds
// small enough that ttl*1000 cannot overflow.
func (i *instance) kvTTL(opts goja.Value) int64 {
	if goja.IsUndefined(opts) || goja.IsNull(opts) {
		return 0
	}
	obj, ok := opts.(*goja.Object)
	if !ok {
		panic(i.vm.NewTypeError("kv.set: options must be an object"))
	}
	raw := obj.Get("ttl")
	if raw == nil || goja.IsUndefined(raw) || goja.IsNull(raw) {
		return 0
	}
	seconds, ok := raw.Export().(float64)
	if !ok {
		if n, isInt := raw.Export().(int64); isInt {
			seconds = float64(n)
		} else {
			panic(i.vm.NewTypeError("kv.set: ttl must be a non-negative whole number of seconds"))
		}
	}
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 ||
		seconds != math.Trunc(seconds) || seconds > maxKVTTLSeconds {
		panic(i.vm.NewTypeError("kv.set: ttl must be a non-negative whole number of seconds"))
	}
	return int64(seconds)
}
