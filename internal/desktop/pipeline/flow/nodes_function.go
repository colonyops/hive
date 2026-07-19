package flow

import (
	"fmt"
	"strings"
	"time"
)

const (
	// minOutputs/maxOutputs bound the function node's outputs field.
	minOutputs = 1
	maxOutputs = 16
	// defaultOutputs is used when outputs is omitted.
	defaultOutputs = 1

	// minFunctionTimeout/maxFunctionTimeout bound an explicit timeout.
	minFunctionTimeout = 100 * time.Millisecond
	maxFunctionTimeout = 60 * time.Second
	// DefaultFunctionTimeout is used when timeout is omitted.
	DefaultFunctionTimeout = 5 * time.Second
)

// FunctionConfig is a function node: 1 input, N outputs (default 1). It
// carries the node's JavaScript lifecycle hooks, evaluated by the frontend
// graph runtime (part B/Phase 6) — this package only parses and validates
// the config.
type FunctionConfig struct {
	OnMessage string   `json:"on_message"         yaml:"on_message"`
	OnStart   string   `json:"on_start,omitempty" yaml:"on_start,omitempty"`
	OnStop    string   `json:"on_stop,omitempty"  yaml:"on_stop,omitempty"`
	OutputsN  int      `json:"outputs,omitempty"  yaml:"outputs,omitempty"`
	Timeout   Duration `json:"timeout,omitempty"  yaml:"timeout,omitempty"`
}

func (c *FunctionConfig) Inputs() int { return 1 }

// Outputs returns the configured output count, defaulting to 1 when unset.
func (c *FunctionConfig) Outputs() int {
	if c.OutputsN <= 0 {
		return defaultOutputs
	}
	return c.OutputsN
}

// EffectiveTimeout returns the configured timeout, defaulting to
// DefaultFunctionTimeout when unset.
func (c *FunctionConfig) EffectiveTimeout() time.Duration {
	if d := c.Timeout.Duration(); d != 0 {
		return d
	}
	return DefaultFunctionTimeout
}

func (c *FunctionConfig) Validate(Refs) error {
	if strings.TrimSpace(c.OnMessage) == "" {
		return fmt.Errorf("function: on_message is required")
	}
	if c.OutputsN != 0 && (c.OutputsN < minOutputs || c.OutputsN > maxOutputs) {
		return fmt.Errorf("function: outputs must be between %d and %d, got %d", minOutputs, maxOutputs, c.OutputsN)
	}
	if d := c.Timeout.Duration(); d != 0 && (d < minFunctionTimeout || d > maxFunctionTimeout) {
		return fmt.Errorf("function: timeout %s out of range [%s, %s]", d, minFunctionTimeout, maxFunctionTimeout)
	}
	return nil
}
