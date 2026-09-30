package js

import (
	"strings"

	"github.com/dop251/goja"
)

// consoleLevels are the console methods bound on the VM's global object. They
// all format identically and differ only in the level they report, which is
// what a dry-run trace groups by.
var consoleLevels = []string{"log", "info", "warn", "error", "debug", "trace"}

// maxConsoleLineBytes truncates one formatted line. A script that stringifies a
// large payload every message would otherwise let a dry-run response grow
// without bound; the marker makes the truncation visible rather than silent.
const maxConsoleLineBytes = 8192

// bindConsole installs the `console` global once per instance. Like the kv
// façade it resolves the live invocation's sink at call time, so a reference a
// script stashed in `state` on one message still writes to the right place on
// the next — and writes nowhere at all when the run supplied no sink, which is
// every live run.
//
// Binding it unconditionally is the point: before this existed, a `console.log`
// left in an author's script threw ReferenceError and dropped the message.
func (i *instance) bindConsole() error {
	obj := i.vm.NewObject()
	for _, level := range consoleLevels {
		if err := obj.Set(level, i.consoleFunc(level)); err != nil {
			return err
		}
	}
	return i.vm.Set("console", obj)
}

func (i *instance) consoleFunc(level string) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if i.curConsole == nil {
			return goja.Undefined()
		}
		i.curConsole.Log(level, i.formatConsoleArgs(call.Arguments))
		return goja.Undefined()
	}
}

// formatConsoleArgs joins the arguments with spaces, rendering strings verbatim
// and everything else as JSON — Node's console shape, minus format specifiers.
// A value JSON.stringify cannot represent (a function, undefined) falls back to
// goja's own string form so it reads as something rather than vanishing.
func (i *instance) formatConsoleArgs(args []goja.Value) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, i.formatConsoleArg(arg))
	}
	return truncate(strings.Join(parts, " "), maxConsoleLineBytes)
}

func (i *instance) formatConsoleArg(arg goja.Value) string {
	if arg == nil || goja.IsUndefined(arg) {
		return "undefined"
	}
	if s, ok := arg.Export().(string); ok {
		return s
	}
	// A throwing toJSON, or a cyclic object, must not turn a debug log into a
	// failed message — fall back rather than propagating the panic.
	encoded, err := i.json.stringify(goja.Undefined(), arg)
	if err != nil || encoded == nil || goja.IsUndefined(encoded) {
		return arg.String()
	}
	return encoded.String()
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "… (truncated)"
}
