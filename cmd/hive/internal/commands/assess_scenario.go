package commands

import (
	"fmt"
	"strings"

	"github.com/colonyops/hive/internal/core/terminal"
	"gopkg.in/yaml.v3"
)

// scenarioStepKind identifies which of a scenarioStep's mutually exclusive
// actions is set.
type scenarioStepKind string

const (
	scenarioStepSend   scenarioStepKind = "send"
	scenarioStepKey    scenarioStepKind = "key"
	scenarioStepExpect scenarioStepKind = "expect"
)

// scenarioSpec is a parsed `hive x assess scenario` YAML file: a named,
// ordered list of steps driving and observing one tmux pane.
type scenarioSpec struct {
	Name  string
	Tool  string // assess.Snapshot.Tool override; empty auto-detects per poll, like `replay --tool`
	Steps []scenarioStep
}

// scenarioStep is one step of a scenario: exactly one of Send, Key, or
// Expect is populated, selected by Kind. UnmarshalYAML enforces "exactly
// one" at parse time so runner/scorer code never has to guess.
type scenarioStep struct {
	Kind   scenarioStepKind
	Send   string // literal text + Enter
	Key    string // one named key (tmux send-keys key name, e.g. "Down", "Enter", "C-c")
	Expect scenarioExpect
}

// scenarioExpect is a windowed expectation: the published status named by
// State must appear within WithinPolls polls of this step starting, and
// then (if HoldsForPolls > 1) remain published for that many consecutive
// polls counting the detection poll itself.
type scenarioExpect struct {
	State         terminal.Status
	WithinPolls   int
	HoldsForPolls int
}

// expectStateAliases maps scenario YAML `state:` tokens to terminal.Status.
// "working" is accepted alongside "active" — assess.StateWorking is the
// Stage-1 vocabulary operators think in, but the tracker only ever
// *publishes* terminal.StatusActive, which is what expectations are scored
// against; the alias lets scenario authors write either.
var expectStateAliases = map[string]terminal.Status{
	"working":  terminal.StatusActive,
	"active":   terminal.StatusActive,
	"approval": terminal.StatusApproval,
	"question": terminal.StatusQuestion,
	"ready":    terminal.StatusReady,
	"missing":  terminal.StatusMissing,
}

// parseScenario parses and validates a scenario YAML document.
func parseScenario(data []byte) (*scenarioSpec, error) {
	var spec scenarioSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parsing scenario: %w", err)
	}
	if spec.Name == "" {
		return nil, fmt.Errorf("scenario: \"scenario\" (name) is required")
	}
	if len(spec.Steps) == 0 {
		return nil, fmt.Errorf("scenario %q: at least one step is required", spec.Name)
	}
	return &spec, nil
}

// UnmarshalYAML supports scenarioSpec's top-level shape:
//
//	scenario: <name>
//	tool: <tool>       # optional
//	steps: [...]
func (s *scenarioSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("scenario: expected a mapping, got %v", node.Kind)
	}

	for i := 0; i < len(node.Content)-1; i += 2 {
		key := node.Content[i].Value
		val := node.Content[i+1]

		switch key {
		case "scenario":
			if err := val.Decode(&s.Name); err != nil {
				return fmt.Errorf("scenario.scenario: %w", err)
			}
		case "tool":
			if err := val.Decode(&s.Tool); err != nil {
				return fmt.Errorf("scenario.tool: %w", err)
			}
		case "steps":
			if err := val.Decode(&s.Steps); err != nil {
				return fmt.Errorf("scenario.steps: %w", err)
			}
		default:
			return fmt.Errorf("scenario: unknown top-level field %q (expected one of: scenario, tool, steps)", key)
		}
	}

	return nil
}

// UnmarshalYAML enforces that a step has exactly one kind (send, key, or
// expect); zero or multiple kinds, or an unrecognized key, are parse errors
// with the offending key name(s) so a scenario author sees exactly what to
// fix.
func (s *scenarioStep) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("step: expected a mapping, got %v", node.Kind)
	}
	if len(node.Content) == 0 {
		return fmt.Errorf("step: empty step (expected one of: send, key, expect)")
	}

	var kinds []string
	for i := 0; i < len(node.Content)-1; i += 2 {
		key := node.Content[i].Value
		val := node.Content[i+1]

		switch key {
		case "send":
			var v string
			if err := val.Decode(&v); err != nil {
				return fmt.Errorf("step.send: %w", err)
			}
			s.Kind, s.Send = scenarioStepSend, v
		case "key":
			var v string
			if err := val.Decode(&v); err != nil {
				return fmt.Errorf("step.key: %w", err)
			}
			s.Kind, s.Key = scenarioStepKey, v
		case "expect":
			exp, err := parseScenarioExpect(val)
			if err != nil {
				return fmt.Errorf("step.expect: %w", err)
			}
			s.Kind, s.Expect = scenarioStepExpect, exp
		default:
			return fmt.Errorf("step: unknown step kind %q (expected one of: send, key, expect)", key)
		}
		kinds = append(kinds, key)
	}

	if len(kinds) > 1 {
		return fmt.Errorf("step: has multiple kinds (%s); each step must have exactly one of send, key, expect", strings.Join(kinds, ", "))
	}

	return nil
}

// parseScenarioExpect parses one `expect: {state, within_polls,
// holds_for_polls}` mapping. holds_for_polls defaults to 1 (satisfied the
// instant the state is first observed).
func parseScenarioExpect(node *yaml.Node) (scenarioExpect, error) {
	if node.Kind != yaml.MappingNode {
		return scenarioExpect{}, fmt.Errorf("expected a mapping with state/within_polls/holds_for_polls, got %v", node.Kind)
	}

	exp := scenarioExpect{HoldsForPolls: 1}
	var haveState, haveWithinPolls bool

	for i := 0; i < len(node.Content)-1; i += 2 {
		key := node.Content[i].Value
		val := node.Content[i+1]

		switch key {
		case "state":
			var raw string
			if err := val.Decode(&raw); err != nil {
				return exp, fmt.Errorf("state: %w", err)
			}
			status, ok := expectStateAliases[raw]
			if !ok {
				return exp, fmt.Errorf("state: unrecognized status %q (expected one of: working/active, approval, question, ready, missing)", raw)
			}
			exp.State = status
			haveState = true
		case "within_polls":
			n, err := decodePositiveInt(val, "within_polls")
			if err != nil {
				return exp, err
			}
			exp.WithinPolls = n
			haveWithinPolls = true
		case "holds_for_polls":
			n, err := decodePositiveInt(val, "holds_for_polls")
			if err != nil {
				return exp, err
			}
			exp.HoldsForPolls = n
		default:
			return exp, fmt.Errorf("unknown field %q (expected one of: state, within_polls, holds_for_polls)", key)
		}
	}

	if !haveState {
		return exp, fmt.Errorf("state is required")
	}
	if !haveWithinPolls {
		return exp, fmt.Errorf("within_polls is required")
	}

	return exp, nil
}

func decodePositiveInt(node *yaml.Node, field string) (int, error) {
	var n int
	if err := node.Decode(&n); err != nil {
		return 0, fmt.Errorf("%s: %w", field, err)
	}
	if n < 1 {
		return 0, fmt.Errorf("%s must be >= 1, got %d", field, n)
	}
	return n, nil
}

// scenarioPoll is one poll captured while a scenario runs: the published
// status observed at that instant, and which step (by index into
// scenarioSpec.Steps) the runner was waiting on. The live runner only polls
// while executing an `expect` step, so in practice every poll belongs to
// exactly one step's window and StepIndex is always >= 0; -1 is supported
// defensively (such a poll is never exempted from flap counting, since it
// isn't implied by any expectation).
type scenarioPoll struct {
	Poll      int
	Published terminal.Status
	StepIndex int
}

// expectationResult is one expect step's scored outcome.
type expectationResult struct {
	StepIndex      int             `json:"stepIndex"`
	Expected       terminal.Status `json:"expected"`
	Pass           bool            `json:"pass"`
	DetectedAtPoll int             `json:"detectedAtPoll"` // 0-based offset into this step's window; -1 if never observed
	Reason         string          `json:"reason,omitempty"`
}

// scenarioReport is the JSON emitted by `hive x assess scenario`.
type scenarioReport struct {
	Scenario     string              `json:"scenario"`
	Pass         bool                `json:"pass"` // all expectations passed; independent of FlapCount (see scoreScenario)
	Expectations []expectationResult `json:"expectations"`
	FlapCount    int                 `json:"flapCount"`
	TotalPolls   int                 `json:"totalPolls"`
}

// scoreScenario is the pure core of `hive x assess scenario`: given the
// scenario definition and the ordered poll log the live runner produced, it
// scores every expectation and counts flaps. It touches no tmux, no clock,
// and no I/O, which is what makes it unit-testable against synthetic logs.
//
// Pass is deliberately independent of FlapCount: a scenario can satisfy
// every expectation while still flapping on the way there (settling into
// the right state only after some noise), and that noise is exactly what
// the flap count exists to surface — folding it into Pass would let a
// tuning change silently trade a passing-but-flappy result for a different
// passing-but-flappy result. Callers (the report reader, the autocalibrate
// loop) treat FlapCount as a separate, ideally-zero signal.
func scoreScenario(spec *scenarioSpec, log []scenarioPoll) scenarioReport {
	report := scenarioReport{Scenario: spec.Name, TotalPolls: len(log), Pass: true}

	// exemptPolls holds the poll numbers of "detection" transitions: the
	// first poll in an expectation's window where the published status
	// became that expectation's target. Every other published-status
	// change anywhere in the log is a flap.
	exemptPolls := make(map[int]bool)

	for stepIdx, step := range spec.Steps {
		if step.Kind != scenarioStepExpect {
			continue
		}

		window := pollsForStep(log, stepIdx)
		result := scoreExpectation(stepIdx, step.Expect, window)
		if !result.Pass {
			report.Pass = false
		}
		if result.DetectedAtPoll >= 0 {
			exemptPolls[window[result.DetectedAtPoll].Poll] = true
		}
		report.Expectations = append(report.Expectations, result)
	}

	report.FlapCount = countFlaps(log, exemptPolls)
	return report
}

// pollsForStep filters log to the polls gathered while awaiting step
// stepIdx's expectation, preserving order.
func pollsForStep(log []scenarioPoll, stepIdx int) []scenarioPoll {
	var window []scenarioPoll
	for _, p := range log {
		if p.StepIndex == stepIdx {
			window = append(window, p)
		}
	}
	return window
}

// scoreExpectation checks one expectation against its poll window: the
// target state must first appear within WithinPolls polls, and then persist
// for HoldsForPolls consecutive polls counting the detection poll itself.
func scoreExpectation(stepIdx int, exp scenarioExpect, window []scenarioPoll) expectationResult {
	result := expectationResult{StepIndex: stepIdx, Expected: exp.State, DetectedAtPoll: -1}

	searchLimit := exp.WithinPolls
	if searchLimit > len(window) {
		searchLimit = len(window)
	}

	detectedAt := -1
	for i := 0; i < searchLimit; i++ {
		if window[i].Published == exp.State {
			detectedAt = i
			break
		}
	}

	if detectedAt < 0 {
		result.Reason = fmt.Sprintf("expected %s within %d poll(s), never observed (%d poll(s) in window)",
			exp.State, exp.WithinPolls, len(window))
		return result
	}
	result.DetectedAtPoll = detectedAt

	holdEnd := detectedAt + exp.HoldsForPolls
	if holdEnd > len(window) {
		result.Reason = fmt.Sprintf("detected %s at poll %d but the window ended before holds_for_polls=%d could be confirmed",
			exp.State, detectedAt, exp.HoldsForPolls)
		return result
	}

	for i := detectedAt; i < holdEnd; i++ {
		if window[i].Published != exp.State {
			result.Reason = fmt.Sprintf("detected %s at poll %d but it reverted to %s before holds_for_polls=%d was satisfied",
				exp.State, detectedAt, window[i].Published, exp.HoldsForPolls)
			return result
		}
	}

	result.Pass = true
	return result
}

// countFlaps counts published-status changes in log that are not one of the
// scenario's expected "detection" transitions (see scoreScenario).
func countFlaps(log []scenarioPoll, exemptPolls map[int]bool) int {
	flaps := 0
	for i := 1; i < len(log); i++ {
		if log[i].Published == log[i-1].Published {
			continue
		}
		if exemptPolls[log[i].Poll] {
			continue
		}
		flaps++
	}
	return flaps
}
