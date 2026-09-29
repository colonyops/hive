package commands

import (
	"testing"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validScenarioYAML = `
scenario: claude-permission-flow
tool: claude
steps:
  - send: "run ` + "`rm /tmp/nonexistent-test`" + `"
  - expect: {state: approval, within_polls: 3, holds_for_polls: 2}
  - key: Down
  - key: Enter
  - expect: {state: working, within_polls: 2}
`

func TestParseScenario_Valid(t *testing.T) {
	spec, err := parseScenario([]byte(validScenarioYAML))
	require.NoError(t, err)

	assert.Equal(t, "claude-permission-flow", spec.Name)
	assert.Equal(t, "claude", spec.Tool)
	require.Len(t, spec.Steps, 5)

	assert.Equal(t, scenarioStepSend, spec.Steps[0].Kind)
	assert.Equal(t, "run `rm /tmp/nonexistent-test`", spec.Steps[0].Send)

	assert.Equal(t, scenarioStepExpect, spec.Steps[1].Kind)
	assert.Equal(t, terminal.StatusApproval, spec.Steps[1].Expect.State)
	assert.Equal(t, 3, spec.Steps[1].Expect.WithinPolls)
	assert.Equal(t, 2, spec.Steps[1].Expect.HoldsForPolls)

	assert.Equal(t, scenarioStepKey, spec.Steps[2].Kind)
	assert.Equal(t, "Down", spec.Steps[2].Key)
	assert.Equal(t, scenarioStepKey, spec.Steps[3].Kind)
	assert.Equal(t, "Enter", spec.Steps[3].Key)

	assert.Equal(t, scenarioStepExpect, spec.Steps[4].Kind)
	assert.Equal(t, terminal.StatusActive, spec.Steps[4].Expect.State) // "working" aliases to StatusActive
	assert.Equal(t, 2, spec.Steps[4].Expect.WithinPolls)
	assert.Equal(t, 1, spec.Steps[4].Expect.HoldsForPolls) // default
}

func TestParseScenario_MissingName(t *testing.T) {
	_, err := parseScenario([]byte(`
steps:
  - key: Enter
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scenario")
}

func TestParseScenario_NoSteps(t *testing.T) {
	_, err := parseScenario([]byte(`
scenario: empty
steps: []
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one step")
}

func TestParseScenario_UnknownTopLevelField(t *testing.T) {
	_, err := parseScenario([]byte(`
scenario: bogus
bogus_field: 1
steps:
  - key: Enter
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus_field")
}

func TestParseScenario_UnknownStepKind(t *testing.T) {
	_, err := parseScenario([]byte(`
scenario: bad-step
steps:
  - frobnicate: true
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown step kind")
	assert.Contains(t, err.Error(), "frobnicate")
}

func TestParseScenario_MultipleKindsInOneStep(t *testing.T) {
	_, err := parseScenario([]byte(`
scenario: bad-step
steps:
  - send: "hello"
    key: Down
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple kinds")
}

func TestParseScenario_EmptyStep(t *testing.T) {
	_, err := parseScenario([]byte(`
scenario: bad-step
steps:
  - {}
`))
	require.Error(t, err)
}

func TestParseScenario_ExpectMissingState(t *testing.T) {
	_, err := parseScenario([]byte(`
scenario: bad-expect
steps:
  - expect: {within_polls: 3}
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state is required")
}

func TestParseScenario_ExpectMissingWithinPolls(t *testing.T) {
	_, err := parseScenario([]byte(`
scenario: bad-expect
steps:
  - expect: {state: approval}
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "within_polls is required")
}

func TestParseScenario_ExpectUnknownState(t *testing.T) {
	_, err := parseScenario([]byte(`
scenario: bad-expect
steps:
  - expect: {state: bogus, within_polls: 1}
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unrecognized status")
}

func TestParseScenario_ExpectNonPositiveWithinPolls(t *testing.T) {
	_, err := parseScenario([]byte(`
scenario: bad-expect
steps:
  - expect: {state: approval, within_polls: 0}
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "within_polls must be >= 1")
}

func TestParseScenario_ExpectNonPositiveHoldsForPolls(t *testing.T) {
	_, err := parseScenario([]byte(`
scenario: bad-expect
steps:
  - expect: {state: approval, within_polls: 1, holds_for_polls: 0}
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "holds_for_polls must be >= 1")
}

func TestParseScenario_ExpectUnknownField(t *testing.T) {
	_, err := parseScenario([]byte(`
scenario: bad-expect
steps:
  - expect: {state: approval, within_polls: 1, bogus: true}
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

func TestParseScenario_ExpectNotAMapping(t *testing.T) {
	_, err := parseScenario([]byte(`
scenario: bad-expect
steps:
  - expect: "approval"
`))
	require.Error(t, err)
}

// --- scoreScenario ---

func poll(n int, status terminal.Status, step int) scenarioPoll {
	return scenarioPoll{Poll: n, Published: status, StepIndex: step}
}

func TestScoreScenario_ImmediateDetectionPasses(t *testing.T) {
	spec := &scenarioSpec{
		Name: "immediate",
		Steps: []scenarioStep{
			{Kind: scenarioStepExpect, Expect: scenarioExpect{State: terminal.StatusApproval, WithinPolls: 3, HoldsForPolls: 1}},
		},
	}
	log := []scenarioPoll{
		poll(0, terminal.StatusApproval, 0),
	}

	report := scoreScenario(spec, log)

	require.Len(t, report.Expectations, 1)
	assert.True(t, report.Expectations[0].Pass)
	assert.Equal(t, 0, report.Expectations[0].DetectedAtPoll)
	assert.True(t, report.Pass)
	assert.Equal(t, 0, report.FlapCount)
}

func TestScoreScenario_DetectionLatencyReported(t *testing.T) {
	spec := &scenarioSpec{
		Name: "latent",
		Steps: []scenarioStep{
			{Kind: scenarioStepExpect, Expect: scenarioExpect{State: terminal.StatusApproval, WithinPolls: 3, HoldsForPolls: 1}},
		},
	}
	log := []scenarioPoll{
		poll(0, terminal.StatusActive, 0),
		poll(1, terminal.StatusActive, 0),
		poll(2, terminal.StatusApproval, 0),
	}

	report := scoreScenario(spec, log)

	require.Len(t, report.Expectations, 1)
	assert.True(t, report.Expectations[0].Pass)
	assert.Equal(t, 2, report.Expectations[0].DetectedAtPoll)
}

func TestScoreScenario_NeverObservedFails(t *testing.T) {
	spec := &scenarioSpec{
		Name: "never",
		Steps: []scenarioStep{
			{Kind: scenarioStepExpect, Expect: scenarioExpect{State: terminal.StatusApproval, WithinPolls: 2, HoldsForPolls: 1}},
		},
	}
	log := []scenarioPoll{
		poll(0, terminal.StatusActive, 0),
		poll(1, terminal.StatusActive, 0),
	}

	report := scoreScenario(spec, log)

	require.Len(t, report.Expectations, 1)
	assert.False(t, report.Expectations[0].Pass)
	assert.Equal(t, -1, report.Expectations[0].DetectedAtPoll)
	assert.NotEmpty(t, report.Expectations[0].Reason)
	assert.False(t, report.Pass)
}

func TestScoreScenario_DetectedOutsideWithinPollsFails(t *testing.T) {
	spec := &scenarioSpec{
		Name: "late",
		Steps: []scenarioStep{
			{Kind: scenarioStepExpect, Expect: scenarioExpect{State: terminal.StatusApproval, WithinPolls: 1, HoldsForPolls: 1}},
		},
	}
	log := []scenarioPoll{
		poll(0, terminal.StatusActive, 0),
		poll(1, terminal.StatusApproval, 0), // arrives one poll too late
	}

	report := scoreScenario(spec, log)

	require.Len(t, report.Expectations, 1)
	assert.False(t, report.Expectations[0].Pass)
	assert.Equal(t, -1, report.Expectations[0].DetectedAtPoll)
}

func TestScoreScenario_HoldsForPollsSatisfied(t *testing.T) {
	spec := &scenarioSpec{
		Name: "holds",
		Steps: []scenarioStep{
			{Kind: scenarioStepExpect, Expect: scenarioExpect{State: terminal.StatusApproval, WithinPolls: 3, HoldsForPolls: 2}},
		},
	}
	log := []scenarioPoll{
		poll(0, terminal.StatusApproval, 0),
		poll(1, terminal.StatusApproval, 0),
	}

	report := scoreScenario(spec, log)

	require.Len(t, report.Expectations, 1)
	assert.True(t, report.Expectations[0].Pass)
}

func TestScoreScenario_HoldsForPollsRevertedFails(t *testing.T) {
	spec := &scenarioSpec{
		Name: "reverts",
		Steps: []scenarioStep{
			{Kind: scenarioStepExpect, Expect: scenarioExpect{State: terminal.StatusApproval, WithinPolls: 3, HoldsForPolls: 2}},
		},
	}
	log := []scenarioPoll{
		poll(0, terminal.StatusApproval, 0),
		poll(1, terminal.StatusActive, 0), // reverted before the hold was confirmed
	}

	report := scoreScenario(spec, log)

	require.Len(t, report.Expectations, 1)
	assert.False(t, report.Expectations[0].Pass)
	assert.Equal(t, 0, report.Expectations[0].DetectedAtPoll)
	assert.Contains(t, report.Expectations[0].Reason, "reverted")
}

func TestScoreScenario_HoldsForPollsExhaustsWindowFails(t *testing.T) {
	spec := &scenarioSpec{
		Name: "short-window",
		Steps: []scenarioStep{
			{Kind: scenarioStepExpect, Expect: scenarioExpect{State: terminal.StatusApproval, WithinPolls: 2, HoldsForPolls: 5}},
		},
	}
	log := []scenarioPoll{
		poll(0, terminal.StatusApproval, 0),
	}

	report := scoreScenario(spec, log)

	require.Len(t, report.Expectations, 1)
	assert.False(t, report.Expectations[0].Pass)
	assert.Contains(t, report.Expectations[0].Reason, "holds_for_polls")
}

// TestScoreScenario_UnexpectedTransitionIncrementsFlapCount is the
// success-criteria case: an expectation can pass while the log still shows
// an unrelated blip on the way there, and that blip must count as a flap.
func TestScoreScenario_UnexpectedTransitionIncrementsFlapCount(t *testing.T) {
	spec := &scenarioSpec{
		Name: "flappy",
		Steps: []scenarioStep{
			{Kind: scenarioStepExpect, Expect: scenarioExpect{State: terminal.StatusApproval, WithinPolls: 3, HoldsForPolls: 1}},
		},
	}
	log := []scenarioPoll{
		poll(0, terminal.StatusReady, 0),
		poll(1, terminal.StatusActive, 0),   // unexpected transition: ready -> active
		poll(2, terminal.StatusApproval, 0), // detection transition: active -> approval (exempt)
		poll(3, terminal.StatusApproval, 0), // no transition
	}

	report := scoreScenario(spec, log)

	require.Len(t, report.Expectations, 1)
	assert.True(t, report.Expectations[0].Pass, "the expectation itself is still satisfied")
	assert.Equal(t, 2, report.Expectations[0].DetectedAtPoll)
	assert.Equal(t, 1, report.FlapCount, "the ready->active blip is not implied by the scenario and must count as a flap")
	assert.True(t, report.Pass, "Pass reflects expectations only; FlapCount is reported separately")
}

func TestScoreScenario_MultipleExpectationsAcrossSteps(t *testing.T) {
	spec := &scenarioSpec{
		Name: "multi",
		Steps: []scenarioStep{
			{Kind: scenarioStepSend, Send: "do the thing"},
			{Kind: scenarioStepExpect, Expect: scenarioExpect{State: terminal.StatusApproval, WithinPolls: 2, HoldsForPolls: 1}},
			{Kind: scenarioStepKey, Key: "Enter"},
			{Kind: scenarioStepExpect, Expect: scenarioExpect{State: terminal.StatusActive, WithinPolls: 2, HoldsForPolls: 1}},
		},
	}
	log := []scenarioPoll{
		poll(0, terminal.StatusApproval, 1), // step 1's window
		poll(1, terminal.StatusActive, 3),   // step 3's window
	}

	report := scoreScenario(spec, log)

	require.Len(t, report.Expectations, 2)
	assert.Equal(t, 1, report.Expectations[0].StepIndex)
	assert.True(t, report.Expectations[0].Pass)
	assert.Equal(t, 3, report.Expectations[1].StepIndex)
	assert.True(t, report.Expectations[1].Pass)
	assert.Equal(t, 0, report.FlapCount)
	assert.True(t, report.Pass)
}

func TestScoreScenario_NoExpectations(t *testing.T) {
	spec := &scenarioSpec{
		Name: "no-expects",
		Steps: []scenarioStep{
			{Kind: scenarioStepSend, Send: "hello"},
		},
	}

	report := scoreScenario(spec, nil)

	assert.Empty(t, report.Expectations)
	assert.True(t, report.Pass)
	assert.Equal(t, 0, report.FlapCount)
}
