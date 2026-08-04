package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/internal/hive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestAssessScenarioCmd_MissingArgErrors(t *testing.T) {
	flags := &Flags{}
	cmd := NewExperimentalCmd(flags, &hive.App{})

	app := &cli.Command{Name: "hive"}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "x", "assess", "scenario"})
	require.Error(t, err)
}

// TestRunAssessScenarioCmd_RefusesWithoutAllowHost proves the interlock runs
// before the scenario file is even read: a fake resolver reporting the host
// default socket must produce the container-only refusal, with no real
// tmux call ever made.
func TestRunAssessScenarioCmd_RefusesWithoutAllowHost(t *testing.T) {
	resolve := func(_ context.Context, _ string) (string, error) {
		return "/tmp/tmux-501/default", nil
	}

	var buf bytes.Buffer
	err := runAssessScenarioCmd(context.Background(), &buf, "/nonexistent/scenario.yaml", "mypane:0.0", false, 0, nil, resolve)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mise container")
	assert.Contains(t, err.Error(), "--allow-host")
}

// TestRunAssessScenarioCmd_AllowHostNeverCallsResolver proves --allow-host
// skips socket resolution entirely, then lets the run fail harmlessly at the
// scenario-file stage instead of ever reaching real tmux.
func TestRunAssessScenarioCmd_AllowHostNeverCallsResolver(t *testing.T) {
	resolve := func(_ context.Context, _ string) (string, error) {
		t.Fatal("resolve must not be called when --allow-host is set")
		return "", nil
	}

	var buf bytes.Buffer
	err := runAssessScenarioCmd(context.Background(), &buf, "/nonexistent/scenario.yaml", "mypane:0.0", true, 0, nil, resolve)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading scenario file")
}

// TestRunAssessScenarioCmd_MalformedScenarioErrorsBeforeAnyTmuxCall proves a
// scenario parse error surfaces before runScenario is ever reached (and
// therefore before any real tmux call), by using a resolver and a scenario
// file that would both explode if touched past the parse step.
func TestRunAssessScenarioCmd_MalformedScenarioErrorsBeforeAnyTmuxCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte("scenario: bad\nsteps:\n  - frobnicate: true\n"), 0o600))

	resolve := func(_ context.Context, _ string) (string, error) {
		t.Fatal("resolve must not be called when --allow-host is set")
		return "", nil
	}

	var buf bytes.Buffer
	err := runAssessScenarioCmd(context.Background(), &buf, path, "mypane:0.0", true, 0, nil, resolve)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown step kind")
}
