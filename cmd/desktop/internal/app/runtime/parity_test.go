package runtime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime/js"
)

// The parity fixtures in testdata/parity are the engine's own regression
// suite: a flow, a batch of event-log messages, and the exact CommitBatch
// that batch is worth. They began as the proof a port was faithful — this
// test ran them through the Go engine while
// desktop/frontend/src/pipeline/engine/__tests__/parity.spec.ts ran the same
// files through the TypeScript engine it replaced, each comparing against the
// same expected commit. That TypeScript engine and its parity spec are long
// gone; this file is now the only thing that runs these fixtures, and a
// change to routing, sink tagging or node-run accounting still belongs in one
// of them (ADR flow-engine-in-go).
//
// Two fields are normalized away before comparing, and only two. durMs is
// wall-clock. err is the engine's own wording for a thrown value — a detail
// of whichever ScriptRuntime ran it, not part of the contract; what has to
// match is *that* the node failed, which ok and the counters already say.

const parityDir = "testdata/parity"

type parityFixture struct {
	Name     string             `json:"name"`
	Why      string             `json:"why"`
	Flow     flow.Flow          `json:"flow"`
	Batch    []models.Msg       `json:"batch"`
	Expected models.CommitBatch `json:"expected"`
}

func TestParityFixtures(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(parityDir)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "no parity fixtures found")

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(filepath.Join(parityDir, entry.Name()))
			require.NoError(t, err)

			var fixture parityFixture
			require.NoError(t, json.Unmarshal(raw, &fixture), "decoding fixture")

			runner, err := runtime.NewRunner(fixture.Flow, runtime.Options{Scripts: testScripts()})
			require.NoError(t, err)
			defer runner.Close()

			got, err := runner.Run(t.Context(), fixture.Batch)
			require.NoError(t, err)

			require.Equal(t, canonical(t, fixture.Expected), canonical(t, got), fixture.Why)
		})
	}
}

// TestParityFixturesAreDeterministic re-runs every fixture on a fresh Runner
// and requires the same bytes. Map iteration is the obvious way for an ordered
// field to drift, and it drifts intermittently — which is exactly the kind of
// thing a single-run assertion happily passes.
func TestParityFixturesAreDeterministic(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(parityDir)
	require.NoError(t, err)

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(parityDir, entry.Name()))
		require.NoError(t, err)

		var fixture parityFixture
		require.NoError(t, json.Unmarshal(raw, &fixture))

		var first string
		for attempt := range 5 {
			runner, err := runtime.NewRunner(fixture.Flow, runtime.Options{Scripts: testScripts()})
			require.NoError(t, err)
			got, err := runner.Run(t.Context(), fixture.Batch)
			require.NoError(t, err)
			runner.Close()

			encoded, err := json.Marshal(canonical(t, got))
			require.NoError(t, err)
			if attempt == 0 {
				first = string(encoded)
				continue
			}
			require.JSONEq(t, first, string(encoded), "%s is not deterministic", entry.Name())
		}
	}
}

func testScripts() *runtime.ScriptRegistry {
	registry := runtime.NewScriptRegistry()
	registry.Register(js.New(runtime.NewScriptPool(0)))
	return registry
}

// canonical renders a commit batch as plain JSON values, so comparison is
// structural rather than byte-for-byte: a payload's key order and whitespace
// are whatever produced it, and neither is part of the contract.
func canonical(t *testing.T, batch models.CommitBatch) map[string]any {
	t.Helper()

	normalized := batch
	normalized.Outputs = orEmpty(batch.Outputs)
	normalized.FeedSnapshots = orEmpty(batch.FeedSnapshots)
	normalized.Discards = orEmpty(batch.Discards)
	normalized.NodeRuns = orEmpty(batch.NodeRuns)
	for i := range normalized.NodeRuns {
		normalized.NodeRuns[i].DurMs = 0
		if normalized.NodeRuns[i].Err != "" {
			normalized.NodeRuns[i].Err = "!"
		}
	}

	encoded, err := json.Marshal(normalized)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(encoded, &out))
	return out
}

// orEmpty replaces a nil slice with an empty one. nil and [] are the same
// thing to every consumer here, and telling them apart would only make the
// fixtures record which of the two the engine happened to produce.
func orEmpty[T any](values []T) []T {
	if len(values) == 0 {
		return []T{}
	}
	return append([]T(nil), values...)
}
