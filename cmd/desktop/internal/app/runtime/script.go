package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
)

// ScriptRuntime is a driven port: one language a function node can be written
// in. It is defined here because the engine is its consumer; implementations
// live in their own packages (runtime/js is the goja one) and satisfy it
// structurally.
//
// Adding a language is an implementation plus one Register call. The engine
// never learns a second language's name, so nothing in run.go changes.
type ScriptRuntime interface {
	// Name is the language this runtime registers under.
	Name() string
	// Check reports whether src compiles, without running it. The editor's
	// live syntax check is a core concern, so it goes through the same
	// compiler the engine uses rather than a second parser.
	Check(src string) error
	// New compiles src into an instance with its own isolated state. outputs
	// is the node's declared output-port count: it is what disambiguates a
	// language's overloaded return shapes, so the engine only ever receives
	// port-indexed values.
	New(src string, outputs int) (ScriptInstance, error)
}

// ConsoleSink receives one console.* call from a script. A nil sink discards
// the line, which is what a live run passes: console output is a debugging
// affordance for a dry run, not a second log stream on the hot path. It is
// invoked from the evaluating goroutine, which for a wedged script may be one
// the engine has already abandoned — an implementation must be safe to call
// after the evaluation it belongs to has returned.
type ConsoleSink func(level, text string)

// Log writes one line, tolerating a nil sink so callers never branch.
func (s ConsoleSink) Log(level, text string) {
	if s != nil {
		s(level, text)
	}
}

// ScriptInstance is one node instance's live script: its compiled program and
// the state object that survives across messages for as long as the instance
// does. Not safe for concurrent use — the engine evaluates one message at a
// time per instance.
type ScriptInstance interface {
	// OnMessage evaluates the script for msg and returns its outputs indexed
	// by output port. config is the node's own config, passed as the script's
	// `node` argument; kv is the node's per-message durable-KV handle and
	// console its per-message console sink. A returned error is always a
	// *ScriptError.
	OnMessage(ctx context.Context, msg models.Msg, config any, kv NodeKV, console ConsoleSink) ([][]models.Msg, error)
	// Close releases the instance. An instance whose evaluation timed out may
	// still be running, in which case Close interrupts it and returns without
	// waiting.
	Close()
}

// ScriptErrorKind classifies a script failure. The editor renders every
// language's diagnostics identically, so the vocabulary is fixed here rather
// than being whatever a given engine happens to report.
type ScriptErrorKind string

const (
	// ScriptErrorCompile is a syntax or compile-time failure.
	ScriptErrorCompile ScriptErrorKind = "compile"
	// ScriptErrorRuntime is a value thrown while the script ran.
	ScriptErrorRuntime ScriptErrorKind = "runtime"
	// ScriptErrorTimeout is an evaluation that outlived the node's timeout.
	ScriptErrorTimeout ScriptErrorKind = "timeout"
	// ScriptErrorResult is a return value the language adapter could not
	// resolve into port-indexed messages.
	ScriptErrorResult ScriptErrorKind = "result"
	// ScriptErrorUnavailable is the script pool refusing to start another
	// evaluation because too many are already wedged.
	ScriptErrorUnavailable ScriptErrorKind = "unavailable"
)

// ScriptError is the normalized failure every ScriptRuntime reports. Line and
// Column are 1-based and zero when the language could not attribute the
// failure to a position.
type ScriptError struct {
	Kind    ScriptErrorKind `json:"kind"`
	Message string          `json:"message"`
	Line    int             `json:"line,omitempty"`
	Column  int             `json:"column,omitempty"`
	// Stack is the language's own trace, kept whole for the editor. It is
	// never parsed.
	Stack string `json:"stack,omitempty"`
}

func (e *ScriptError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s", e.Kind, e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

// ScriptRegistry maps a language name to its runtime. It is an explicit
// registry rather than init() self-registration: gochecknoinits is enabled,
// and a registry that is assembled in one visible place is the point.
type ScriptRegistry struct {
	mu       sync.RWMutex
	runtimes map[string]ScriptRuntime
}

// NewScriptRegistry returns an empty registry.
func NewScriptRegistry() *ScriptRegistry {
	return &ScriptRegistry{runtimes: map[string]ScriptRuntime{}}
}

// Register adds rt under its own name. Registering a name twice is a
// programming error and panics at wiring time rather than silently shadowing.
func (r *ScriptRegistry) Register(rt ScriptRuntime) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.runtimes[rt.Name()]; exists {
		panic("runtime: script runtime " + rt.Name() + " registered twice")
	}
	r.runtimes[rt.Name()] = rt
}

// Lookup returns the runtime registered under name.
func (r *ScriptRegistry) Lookup(name string) (ScriptRuntime, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.runtimes[name]
	return rt, ok
}

// DefaultScriptLanguage is the language a function node is written in. There
// is no `runtime:` field on the node yet — the port and registry are what make
// a second language possible, and the config field can follow when one exists.
const DefaultScriptLanguage = "javascript"

// DefaultMaxConcurrentScripts bounds how many script evaluations may be in
// flight at once, process-wide.
const DefaultMaxConcurrentScripts = 32

// ScriptPool bounds concurrent script evaluations. One pool is shared by
// every registered ScriptRuntime, which is why it is passed to them at
// construction rather than owned by any one of them.
//
// A cooperative interrupt cannot stop every pathological script — a tight
// loop that neither allocates nor calls back into the host may keep running
// after its timeout fires. A runtime therefore evaluates on a goroutine it is
// willing to abandon, and holds its slot until that goroutine actually
// returns. Bounding the pool converts "one bad node hangs the process" into
// "one bad node exhausts a fixed budget and every later evaluation fails
// fast", which is a limitation the user can see rather than a hang they
// cannot.
type ScriptPool struct {
	slots chan struct{}
}

// NewScriptPool returns a pool admitting at most max concurrent evaluations.
// A non-positive max uses DefaultMaxConcurrentScripts.
func NewScriptPool(maxConcurrent int) *ScriptPool {
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultMaxConcurrentScripts
	}
	return &ScriptPool{slots: make(chan struct{}, maxConcurrent)}
}

// Acquire takes a slot, or reports false immediately when none is free.
// It never blocks: an evaluation that has to queue behind wedged goroutines
// would just add its own timeout to the caller's.
func (p *ScriptPool) Acquire() bool {
	select {
	case p.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release returns a slot. It is only ever called by an evaluation goroutine
// that actually finished.
func (p *ScriptPool) Release() { <-p.slots }

// Available reports how many evaluations could start right now.
func (p *ScriptPool) Available() int { return cap(p.slots) - len(p.slots) }
