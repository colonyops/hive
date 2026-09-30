// Package perf records UI performance spans to a size-capped JSONL file for
// offline analysis. It is a development facility gated on
// development.perf.enabled: a disabled Recorder is a no-op and a shipped build
// never opens a file.
package perf

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	DefaultMaxBytes = 8 << 20

	// FileName is the recorder's file, relative to the state directory.
	FileName = "perf.jsonl"

	maxScopeLen = 64
	maxNameLen  = 128
	maxAttrs    = 32
	maxAttrLen  = 512
)

// ErrDisabled is returned by Record on a Recorder built by Off.
var ErrDisabled = errors.New("perf recording is disabled")

// Sample is one completed span. The shape is OpenTelemetry-compatible so the
// file can be converted to OTLP without re-instrumenting.
type Sample struct {
	At         time.Time      `json:"at"`
	Scope      string         `json:"scope"`
	Name       string         `json:"name"`
	DurationMs float64        `json:"durationMs"`
	Attrs      map[string]any `json:"attrs,omitempty"`
	ID         string         `json:"id,omitempty"`
	Parent     string         `json:"parent,omitempty"`
}

// Validate reports whether the sample is recordable. Attrs is bounded so one
// caller cannot write an unbounded blob into a file nothing rotates per-entry.
func (s Sample) Validate() error {
	switch {
	case s.Scope == "":
		return errors.New("scope is required")
	case len(s.Scope) > maxScopeLen:
		return fmt.Errorf("scope exceeds %d bytes", maxScopeLen)
	case s.Name == "":
		return errors.New("name is required")
	case len(s.Name) > maxNameLen:
		return fmt.Errorf("name exceeds %d bytes", maxNameLen)
	case s.DurationMs < 0:
		return errors.New("duration must not be negative")
	case len(s.Attrs) > maxAttrs:
		return fmt.Errorf("more than %d attributes", maxAttrs)
	}
	for k, v := range s.Attrs {
		if len(k) > maxAttrLen {
			return fmt.Errorf("attribute %q: key exceeds %d bytes", k, maxAttrLen)
		}
		if s, ok := v.(string); ok && len(s) > maxAttrLen {
			return fmt.Errorf("attribute %q: value exceeds %d bytes", k, maxAttrLen)
		}
	}
	return nil
}

// record is what lands on disk. Seq is assigned by the Recorder rather than
// taken from the caller: the frontend cannot produce a monotonic counter that
// survives a page reload, and write order is what makes the file readable.
type record struct {
	Seq uint64 `json:"seq"`
	Sample
}

type Options struct {
	Path     string
	MaxBytes int64
}

// Recorder appends samples to the JSONL file. Safe for concurrent use; a
// batch is written under one lock so its samples land contiguously and the
// sequence matches write order.
type Recorder struct {
	mu   sync.Mutex
	sink *sink
	seq  uint64
}

// New opens (or creates) the recorder's file, appending to whatever is there.
func New(opts Options) (*Recorder, error) {
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	s, err := openSink(opts.Path, opts.MaxBytes)
	if err != nil {
		return nil, err
	}
	return &Recorder{sink: s}, nil
}

// Off returns a Recorder that discards everything and reports Enabled false.
// Callers hold a non-nil *Recorder either way, so no call site needs a nil
// check to stay correct when recording is off.
func Off() *Recorder { return &Recorder{} }

func (r *Recorder) Enabled() bool { return r.sink != nil }

func (r *Recorder) Path() string {
	if r.sink == nil {
		return ""
	}
	return r.sink.path
}

func (r *Recorder) MaxBytes() int64 {
	if r.sink == nil {
		return 0
	}
	return r.sink.max
}

// Record writes the valid samples in the batch and returns how many landed.
// Invalid samples are dropped rather than failing the batch: instrumentation
// added ad hoc must not be able to break a flush with one bad record. A write
// error stops the batch and is returned with the count that made it.
func (r *Recorder) Record(samples ...Sample) (int, error) {
	if r.sink == nil {
		return 0, ErrDisabled
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	written := 0
	for _, s := range samples {
		if s.Validate() != nil {
			continue
		}
		if s.At.IsZero() {
			s.At = time.Now().UTC()
		}
		r.seq++
		if err := r.sink.write(record{Seq: r.seq, Sample: s}); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

func (r *Recorder) Close() error {
	if r.sink == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sink.close()
}
