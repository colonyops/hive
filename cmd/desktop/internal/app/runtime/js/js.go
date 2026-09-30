// Package js implements the engine's ScriptRuntime port with goja, a pure-Go
// ECMAScript interpreter.
//
// Pure Go is a hard requirement, not a preference: the headless `-tags server`
// build is CGO-free (the repo is on modernc.org/sqlite for the same reason),
// so a cgo engine like QuickJS would take that build away.
//
// Values cross the boundary as JSON, through the VM's own JSON.parse and
// JSON.stringify rather than goja's Go-value wrappers. That is what makes the
// round trip faithful: a Go map handed to goja with ToValue is a proxy for
// which Array.isArray reports false and key order is a map's, whereas
// JSON.parse produces genuine JavaScript objects and arrays. null, [], {} and
// key order all survive, which is the property that ruled Lua out.
package js

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
)

// Name is the language this runtime registers under.
const Name = "javascript"

// wrapperPrefix and wrapperSuffix turn a function node's `on_message` body
// into a callable. `new Function('msg','node','state', src)` is what the
// browser did, and this is its goja equivalent — the same author-trusted
// posture, no sandbox.
//
// The prefix ends with a newline so a source line N is line N+1 of the
// compiled program; wrapperLineOffset undoes that when reporting a position,
// so an author sees the line they wrote.
const (
	wrapperPrefix     = "(function (msg, node, state, kv) {\n"
	wrapperSuffix     = "\n})"
	wrapperLineOffset = 1
	// scriptName is the filename goja attributes positions to. It shows up in
	// stack traces, so it names the thing an author recognises.
	scriptName = "on_message"
)

// These caps keep KV suitable for small dedup values, not blob storage.
// Oversized values raise a catchable script error. Key count remains uncapped
// because TTL and flow or node teardown reclaim rows.
const (
	maxKVKeyBytes   = 512
	maxKVValueBytes = 4096
	// maxKVTTLSeconds rejects absurd ttls before ttl*1000 can overflow int64.
	maxKVTTLSeconds = 100 * 365 * 24 * 60 * 60
)

// Runtime is the goja implementation of runtime.ScriptRuntime.
type Runtime struct {
	pool *runtime.ScriptPool
}

// New returns the JavaScript script runtime, bounded by pool. A nil pool gets
// a private one at the default size — convenient for a test, wrong for the
// app, where every runtime shares one budget.
func New(pool *runtime.ScriptPool) *Runtime {
	if pool == nil {
		pool = runtime.NewScriptPool(0)
	}
	return &Runtime{pool: pool}
}

// Name implements runtime.ScriptRuntime.
func (*Runtime) Name() string { return Name }

// Check compiles src without running it, so the editor's syntax check and the
// engine share one compiler.
func (*Runtime) Check(src string) error {
	_, err := compile(src)
	return err
}

// New compiles src into an instance holding its own VM and state object.
func (r *Runtime) New(src string, outputs int) (runtime.ScriptInstance, error) {
	program, err := compile(src)
	if err != nil {
		return nil, err
	}
	if outputs < 1 {
		outputs = 1
	}

	vm := goja.New()
	value, err := vm.RunProgram(program)
	if err != nil {
		return nil, scriptError(runtime.ScriptErrorCompile, err)
	}
	fn, ok := goja.AssertFunction(value)
	if !ok {
		return nil, &runtime.ScriptError{Kind: runtime.ScriptErrorCompile, Message: "on_message did not compile to a function"}
	}

	bridge, err := jsonBridge(vm)
	if err != nil {
		return nil, err
	}

	inst := &instance{
		vm:      vm,
		fn:      fn,
		outputs: outputs,
		json:    bridge,
		pool:    r.pool,
		// The state object lives in the VM for the instance's whole life, so
		// `state.counts ??= {}` on one message is visible to the next — the
		// same object identity the browser worker kept.
		state: vm.NewObject(),
	}
	// The kv façade is likewise built once and lives with the VM: a script
	// may stash `kv` (or a bound method) in `state` on one message and call
	// it on a later one, so the methods resolve the *current* invocation's
	// NodeKV and ctx at call time rather than closing over one message's.
	inst.kvObject = inst.buildKVObject()
	if err := inst.bindConsole(); err != nil {
		return nil, &runtime.ScriptError{Kind: runtime.ScriptErrorCompile, Message: "binding console: " + err.Error()}
	}
	return inst, nil
}

// instance is one node instance's compiled script plus its VM and state.
type instance struct {
	vm       *goja.Runtime
	fn       goja.Callable
	outputs  int
	json     jsonFuncs
	pool     *runtime.ScriptPool
	state    *goja.Object
	kvObject *goja.Object
	// curKV/curCtx/curConsole are the live invocation's KV handle, timeout ctx
	// and console sink, set by OnMessage around each evaluation. nil curKV
	// means kv is unavailable (a runner built without a KV port) and every kv
	// method throws; nil curConsole means console output is discarded, which is
	// every live run. The ctx must live on the struct: the VM's host closures
	// cannot take a Go parameter, and the façades outlive any one invocation by
	// design.
	curKV      runtime.NodeKV
	curCtx     context.Context //nolint:containedctx // invocation-scoped; set/cleared by OnMessage
	curConsole runtime.ConsoleSink
	// wedged records that an evaluation outlived its interrupt and may still
	// be running on an abandoned goroutine. The VM must never be touched
	// again: another Run on it would race that goroutine.
	wedged bool
}

// jsonFuncs are the VM's own JSON.parse and JSON.stringify, resolved once.
type jsonFuncs struct {
	parse     goja.Callable
	stringify goja.Callable
}

func jsonBridge(vm *goja.Runtime) (jsonFuncs, error) {
	obj := vm.Get("JSON")
	if obj == nil {
		return jsonFuncs{}, &runtime.ScriptError{Kind: runtime.ScriptErrorCompile, Message: "JSON is not available in this VM"}
	}
	object, ok := obj.(*goja.Object)
	if !ok {
		return jsonFuncs{}, &runtime.ScriptError{Kind: runtime.ScriptErrorCompile, Message: "JSON is not an object in this VM"}
	}
	parse, ok := goja.AssertFunction(object.Get("parse"))
	if !ok {
		return jsonFuncs{}, &runtime.ScriptError{Kind: runtime.ScriptErrorCompile, Message: "JSON.parse is not callable in this VM"}
	}
	stringify, ok := goja.AssertFunction(object.Get("stringify"))
	if !ok {
		return jsonFuncs{}, &runtime.ScriptError{Kind: runtime.ScriptErrorCompile, Message: "JSON.stringify is not callable in this VM"}
	}
	return jsonFuncs{parse: parse, stringify: stringify}, nil
}

// interruptGrace is how long OnMessage waits for an interrupted evaluation to
// unwind before abandoning its goroutine. goja checks its interrupt flag
// frequently enough that an ordinary runaway loop stops well inside this;
// exceeding it means the script is in a section that cannot be interrupted at
// all, and waiting longer would only extend the caller's stall.
const interruptGrace = 250 * time.Millisecond

// OnMessage evaluates the script for msg and returns its outputs indexed by
// output port.
//
// The evaluation runs on its own goroutine because a cooperative interrupt is
// not guaranteed to land: returning to the caller is what keeps one bad node
// from stopping its flow. A goroutine that outlives its interrupt is
// abandoned and keeps its pool slot, and the instance is marked wedged so the
// engine's reset drops it.
func (i *instance) OnMessage(ctx context.Context, msg models.Msg, config any, kv runtime.NodeKV, console runtime.ConsoleSink) ([][]models.Msg, error) {
	if i.wedged {
		return nil, &runtime.ScriptError{Kind: runtime.ScriptErrorTimeout, Message: "script instance is still running a previous message"}
	}

	// The slot is held until the evaluation goroutine actually returns, which
	// an abandoned one never does. Refusing outright when the budget is spent
	// beats queueing behind a script nothing can stop.
	if !i.pool.Acquire() {
		return nil, &runtime.ScriptError{
			Kind:    runtime.ScriptErrorUnavailable,
			Message: "too many script evaluations are already running; an earlier one is not responding",
		}
	}

	msgValue, err := i.toJS(msg)
	if err != nil {
		i.pool.Release()
		return nil, err
	}
	nodeValue, err := i.toJS(config)
	if err != nil {
		i.pool.Release()
		return nil, err
	}

	type outcome struct {
		value goja.Value
		err   error
	}
	done := make(chan outcome, 1)

	// The interrupt is armed before the goroutine starts so a context that is
	// already done cannot be missed.
	i.vm.ClearInterrupt()
	stop := context.AfterFunc(ctx, func() { i.vm.Interrupt(errTimeout) })
	defer stop()

	// Set before the eval goroutine starts; cleared only on paths where that
	// goroutine has finished. The wedged path deliberately skips the clear —
	// the instance is never reused, and clearing would race the abandoned
	// goroutine.
	i.curKV, i.curCtx, i.curConsole = kv, ctx, console
	clearInvocation := func() { i.curKV, i.curCtx, i.curConsole = nil, nil, nil }

	go func() {
		value, err := i.fn(goja.Undefined(), msgValue, nodeValue, i.state, i.kvObject)
		// done is buffered, so this never blocks even when nobody is left
		// listening — and the slot is returned only now, when the VM is
		// genuinely idle again.
		done <- outcome{value: value, err: err}
		i.pool.Release()
	}()

	select {
	case result := <-done:
		clearInvocation()
		if result.err != nil {
			return nil, evaluationError(result.err)
		}
		return i.fromJS(result.value)
	case <-ctx.Done():
		i.vm.Interrupt(errTimeout)
		select {
		case result := <-done:
			clearInvocation()
			// The interrupt landed. A script that returned a value in the same
			// breath still loses: it ran past its deadline.
			if result.err != nil {
				return nil, evaluationError(result.err)
			}
			return nil, &runtime.ScriptError{Kind: runtime.ScriptErrorTimeout, Message: "script did not complete within its timeout"}
		case <-time.After(interruptGrace):
			i.wedged = true
			return nil, &runtime.ScriptError{Kind: runtime.ScriptErrorTimeout, Message: "script did not respond to interruption and was abandoned"}
		}
	}
}

// Close releases the instance. A wedged instance's goroutine is left to
// whatever it is doing — there is no way to stop it — so Close only makes
// sure nothing else will use its VM.
func (i *instance) Close() {
	if i.wedged {
		i.vm.Interrupt(errTimeout)
		return
	}
	i.vm.ClearInterrupt()
}

// errTimeout is the value handed to Interrupt. goja surfaces it as an
// *goja.InterruptedError whose Value is this error.
var errTimeout = errors.New("script timeout")

// toJS marshals a Go value and parses it inside the VM, so what the script
// sees is an ordinary JavaScript value rather than a proxy for a Go one.
func (i *instance) toJS(v any) (goja.Value, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		return nil, &runtime.ScriptError{Kind: runtime.ScriptErrorResult, Message: fmt.Sprintf("encoding %T for the script: %v", v, err)}
	}
	value, err := i.json.parse(goja.Undefined(), i.vm.ToValue(string(encoded)))
	if err != nil {
		return nil, evaluationError(err)
	}
	return value, nil
}

// fromJS resolves what the script returned into port-indexed messages.
//
// JavaScript's return shapes overlap syntactically — Msg[] and a port-indexed
// array are both plain arrays — so they are disambiguated by the node's
// declared output count, exactly as the browser engine did:
//
//	null / undefined            -> nothing at all (the message is discarded)
//	a single object             -> port 0
//	an array, 1 output          -> several messages, all on port 0
//	an array, N>1 outputs       -> element i is Msg | Msg[] | null for port i
//
// Resolving it here rather than in the engine is what lets a second language
// bring its own ambiguities without the engine learning them.
func (i *instance) fromJS(value goja.Value) ([][]models.Msg, error) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, nil
	}

	encoded, err := i.stringify(value)
	if err != nil {
		return nil, err
	}
	if len(encoded) == 0 || string(encoded) == "null" {
		return nil, nil
	}

	// A non-array result is one message on port 0.
	if encoded[0] != '[' {
		msg, err := decodeMsg(encoded)
		if err != nil {
			return nil, err
		}
		ports := make([][]models.Msg, i.outputs)
		ports[0] = []models.Msg{msg}
		return ports, nil
	}

	var entries []json.RawMessage
	if err := json.Unmarshal(encoded, &entries); err != nil {
		return nil, &runtime.ScriptError{Kind: runtime.ScriptErrorResult, Message: "decoding the returned array: " + err.Error()}
	}

	if i.outputs <= 1 {
		ports := make([][]models.Msg, i.outputs)
		for _, entry := range entries {
			if isJSONNull(entry) {
				continue
			}
			msg, err := decodeMsg(entry)
			if err != nil {
				return nil, err
			}
			ports[0] = append(ports[0], msg)
		}
		return ports, nil
	}

	// A script may address more ports than the node declares. Those messages
	// are not an error: the engine finds no wire on that port and accounts for
	// each one as a discard, which is what happened in the browser and what
	// keeps the node-run counters honest.
	ports := make([][]models.Msg, max(i.outputs, len(entries)))
	for port, entry := range entries {
		if isJSONNull(entry) {
			continue
		}
		if entry[0] == '[' {
			var group []json.RawMessage
			if err := json.Unmarshal(entry, &group); err != nil {
				return nil, &runtime.ScriptError{Kind: runtime.ScriptErrorResult, Message: fmt.Sprintf("decoding port %d: %v", port, err)}
			}
			for _, item := range group {
				if isJSONNull(item) {
					continue
				}
				msg, err := decodeMsg(item)
				if err != nil {
					return nil, err
				}
				ports[port] = append(ports[port], msg)
			}
			continue
		}
		msg, err := decodeMsg(entry)
		if err != nil {
			return nil, err
		}
		ports[port] = []models.Msg{msg}
	}
	return ports, nil
}

// stringify serialises a returned value with the VM's own JSON.stringify.
func (i *instance) stringify(value goja.Value) ([]byte, error) {
	encoded, err := i.json.stringify(goja.Undefined(), value)
	if err != nil {
		return nil, evaluationError(err)
	}
	if encoded == nil || goja.IsUndefined(encoded) {
		return nil, nil
	}
	return []byte(encoded.String()), nil
}

func isJSONNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}

// decodeMsg reads one returned value as a message. The envelope is a fixed
// set of fields: anything else an author attaches to the message object is
// not carried downstream, because the envelope is also what an HTTP or MCP
// surface serialises. Per-message data belongs in msg.Payload.
func decodeMsg(raw json.RawMessage) (models.Msg, error) {
	var msg models.Msg
	if err := json.Unmarshal(raw, &msg); err != nil {
		return models.Msg{}, &runtime.ScriptError{Kind: runtime.ScriptErrorResult, Message: "the returned value is not a message: " + err.Error()}
	}
	return msg, nil
}

// compile builds the callable form of a function body.
//
// It parses before compiling rather than calling goja.Compile, which discards
// the parser's positions on its way to an error string. An author needs the
// line and column of their own mistake, and the editor needs them structured.
func compile(src string) (*goja.Program, error) {
	program, err := parser.ParseFile(nil, scriptName, wrapperPrefix+src+wrapperSuffix, 0)
	if err != nil {
		return nil, syntaxError(err, strings.Count(src, "\n")+1)
	}
	compiled, err := goja.CompileAST(program, false)
	if err != nil {
		return nil, scriptError(runtime.ScriptErrorCompile, err)
	}
	return compiled, nil
}

// syntaxError reports the author's first parse error at their own line.
//
// The reported line is clamped to the source the author actually wrote. An
// unterminated construct is reported at end of input, which sits on the
// wrapper's closing brace — a line the author cannot see, let alone fix.
// Their last line is the useful answer.
func syntaxError(err error, srcLines int) *runtime.ScriptError {
	out := &runtime.ScriptError{Kind: runtime.ScriptErrorCompile, Message: err.Error()}
	var list parser.ErrorList
	if errors.As(err, &list) && len(list) > 0 {
		out.Message = list[0].Message
		out.Line = min(max(list[0].Position.Line-wrapperLineOffset, 1), srcLines)
		out.Column = list[0].Position.Column
	}
	return out
}

// evaluationError classifies a failure goja reported while running.
func evaluationError(err error) error {
	if interrupted, ok := errors.AsType[*goja.InterruptedError](err); ok {
		return &runtime.ScriptError{Kind: runtime.ScriptErrorTimeout, Message: "script did not complete within its timeout", Stack: interrupted.String()}
	}
	return scriptError(runtime.ScriptErrorRuntime, err)
}

// scriptError normalizes a goja failure into the engine's vocabulary,
// translating positions back to the author's own source lines.
func scriptError(kind runtime.ScriptErrorKind, err error) *runtime.ScriptError {
	out := &runtime.ScriptError{Kind: kind, Message: err.Error()}

	if compilerErr, ok := errors.AsType[*goja.CompilerSyntaxError](err); ok {
		out.Message = compilerErr.Message
		if compilerErr.File != nil {
			position := compilerErr.File.Position(compilerErr.Offset)
			out.Line = position.Line - wrapperLineOffset
			out.Column = position.Column
		}
		return out
	}

	if exception, ok := errors.AsType[*goja.Exception](err); ok {
		out.Message = rebasePositions(strings.TrimPrefix(exception.Error(), "Uncaught "))
		out.Stack = rebasePositions(exception.String())
		for _, frame := range exception.Stack() {
			position := frame.Position()
			if position.Filename == scriptName && position.Line > 0 {
				out.Line = position.Line - wrapperLineOffset
				out.Column = position.Column
				break
			}
		}
		return out
	}

	return out
}

// positionRef matches the "on_message:LINE:COLUMN" goja writes into an
// exception's message and stack.
var positionRef = regexp.MustCompile(`\b` + scriptName + `:(\d+):(\d+)`)

// rebasePositions rewrites goja's own line numbers to the author's.
//
// Line and Column are corrected against the wrapper, but the rendered message
// and stack carry goja's raw positions, so the same failure reported "line 2"
// structurally and "on_message:3:28" in prose. Whichever one a reader trusts,
// the other sent them to the wrong line.
func rebasePositions(text string) string {
	return positionRef.ReplaceAllStringFunc(text, func(match string) string {
		groups := positionRef.FindStringSubmatch(match)
		line, err := strconv.Atoi(groups[1])
		if err != nil {
			return match
		}
		return fmt.Sprintf("%s:%d:%s", scriptName, max(line-wrapperLineOffset, 1), groups[2])
	})
}
