//go:build linux || darwin

package classifier_test

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/platform/proc/classifier"
)

const blockingHelperEnv = "HIVE_CLASSIFIER_BLOCKING_HELPER"

func TestBlockingHelper(t *testing.T) {
	if os.Getenv(blockingHelperEnv) == "" {
		t.Skip("runs only as the child of TestClassify_InheritedClaudeCodeEnvIsNotAnAgent")
	}
	time.Sleep(time.Minute)
}

// An agent's CLAUDECODE=1 reaches everything it starts, such as dev servers
// launched through svx. The child is a real process because macOS hides the
// environment of system binaries like sleep, which would pass this test even
// with the variable trusted. Setsid drops the controlling terminal: with one,
// process.Candidates starts from the terminal's foreground group, not the child.
func TestClassify_InheritedClaudeCodeEnvIsNotAnAgent(t *testing.T) {
	child := exec.Command(os.Args[0], "-test.run=^TestBlockingHelper$")
	child.Env = append(os.Environ(), blockingHelperEnv+"=1", "CLAUDECODE=1")
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	require.NoError(t, child.Start())
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})

	input := classifier.PaneInput{PanePID: int64(child.Process.Pid)}

	got := classifier.New(testToolPatterns(testToolClaude), nil, nil, nil).Classify(context.Background(), input)
	assert.False(t, got.IsAgent, "classified as %q at tier %d", got.Tool, got.Tier)

	control := classifier.New(testToolPatterns("classifier"), nil, nil, nil).Classify(context.Background(), input)
	assert.True(t, control.IsAgent, "the classifier must be able to read the child process")
}
