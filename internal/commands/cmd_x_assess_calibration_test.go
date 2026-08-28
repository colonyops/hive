package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/colonyops/hive/internal/core/terminal/status"
	"github.com/stretchr/testify/require"
)

// calibrationSequencesDir holds the committed calibration corpus: paired
// <name>.jsonl frame recordings and <name>.expected.json published-status
// sidecars. See test/calibration/sequences and the autocalibrate skill
// (.claude/skills/autocalibrate/SKILL.md) for how this corpus is grown.
const calibrationSequencesDir = "../../test/calibration/sequences"

// TestCalibrationCorpus_ReplayMatchesExpected is the automated guardrail the
// autocalibrate skill's hard rule ("no regression trading") depends on: it
// replays every committed sequence through a fresh Engine+Tracker on a
// virtual clock — status.DefaultOptions(), never an ambient config file, so
// the result depends only on the shipped debounce defaults and the
// recording, not on whatever poll_interval happens to be configured on the
// machine running the test — and asserts the published-status sequence
// equals that sequence's sidecar exactly. A tuning change that breaks any
// one of these must fail this test before it can land.
func TestCalibrationCorpus_ReplayMatchesExpected(t *testing.T) {
	entries, err := os.ReadDir(calibrationSequencesDir)
	require.NoError(t, err)

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".jsonl"))
	}
	require.NotEmpty(t, names, "calibration corpus must not be empty")

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			framesPath := filepath.Join(calibrationSequencesDir, name+".jsonl")
			expectedPath := filepath.Join(calibrationSequencesDir, name+".expected.json")

			frames, err := readAssessFrames(framesPath)
			require.NoError(t, err)
			require.NotEmpty(t, frames, "sequence must contain at least one frame")

			expectedData, err := os.ReadFile(expectedPath)
			require.NoError(t, err, "missing sidecar %s", expectedPath)
			var expected []terminal.Status
			require.NoError(t, json.Unmarshal(expectedData, &expected))
			require.Len(t, expected, len(frames), "sidecar must have one entry per frame")

			opts := status.DefaultOptions()
			var virtualNow time.Time
			opts.Clock = func() time.Time { return virtualNow }
			tracker := status.NewTracker(assess.NewEngine(), opts)

			got := make([]terminal.Status, 0, len(frames))
			var generation uint64
			for _, f := range frames {
				generation++
				published, _ := observeFrame(tracker, "calibration", f, "", generation, &virtualNow)
				got = append(got, published)
			}

			require.Equal(t, expected, got, "published-status sequence mismatch for %s", name)
		})
	}
}
