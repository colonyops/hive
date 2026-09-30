package actions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultActionsYAMLParsesValidatesAndRenders keeps the shipped starter
// catalog honest. The seed_* tests above only assert byte-identity, so without
// this a malformed or missingkey-tripping default would install cleanly and
// fail every user on first invoke. It parses and validates the bytes, checks
// every registered type is demonstrated, and renders each template through the
// same renderer the executors use (missingkey=error) over the two payload
// shapes a pr/issue action can be invoked against.
func TestDefaultActionsYAMLParsesValidatesAndRenders(t *testing.T) {
	parsed, err := parseCatalog([]byte(defaultActionsYAML))
	require.NoError(t, err)
	require.NotEmpty(t, parsed.Actions)
	// The other list in the file: a starter catalog that demonstrates every
	// action type but no launcher would leave the palette empty on first run.
	require.NotEmpty(t, parsed.Launchers)

	seen := make(map[string]bool, len(parsed.Actions))
	for _, a := range parsed.Actions {
		seen[a.Type] = true
	}
	for _, actionType := range Types() {
		assert.Containsf(t, seen, actionType, "default catalog does not demonstrate type %q", actionType)
	}

	// A search result carries state + labels + author; a notification omits
	// state and reason, blanks author, and sends null labels. A template
	// rendered with missingkey=error must reference no key absent from either.
	payloads := map[string]string{
		"search":       `{"kind":"PR","repo":"colonyops/hive","num":42,"title":"Add retry to the fetch loop","author":"octocat","state":"open","labels":["bug","ci"],"body":"Fixes the flaky fetch.","url":"https://github.com/colonyops/hive/pull/42"}`,
		"notification": `{"kind":"Issue","repo":"colonyops/hive","num":7,"title":"Investigate flaky test","author":"","reason":"mention","labels":null,"body":"GitHub notification for issue in colonyops/hive.","url":"https://github.com/colonyops/hive/issues/7"}`,
	}
	decoded := make(map[string]map[string]any, len(payloads))
	for shape, raw := range payloads {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &m))
		decoded[shape] = m
	}

	renderer := tmpl.New(tmpl.Config{})
	render := func(t *testing.T, name, template string) {
		t.Helper()
		for shape, payload := range decoded {
			out, err := renderer.Render(template, map[string]any{"Payload": payload})
			require.NoErrorf(t, err, "%s over %s payload", name, shape)
			assert.NotEmptyf(t, strings.TrimSpace(out), "%s over %s payload rendered blank", name, shape)
		}
	}

	for _, a := range parsed.Actions {
		switch c := a.Config.(type) {
		case *LaunchSessionConfig:
			render(t, a.ID+".prompt_template", c.PromptTemplate)
			if c.RepoTemplate != "" {
				render(t, a.ID+".repo_template", c.RepoTemplate)
			}
		case *ShellConfig:
			render(t, a.ID+".command_template", c.CommandTemplate)
		case *PublishMessageConfig:
			render(t, a.ID+".message_template", c.MessageTemplate)
		case *ClipboardConfig:
			render(t, a.ID+".text_template", c.TextTemplate)
		default:
			t.Fatalf("action %q: unexpected config type %T", a.ID, c)
		}
	}
}

func TestSeedDefaultsIfMissingInstallsExactBytesOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "actions.yml")
	seeded, err := SeedDefaultsIfMissing(path)
	require.NoError(t, err)
	assert.True(t, seeded)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	if string(got) != defaultActionsYAML {
		t.Fatal("seed bytes differ from exact default catalog")
	}

	seeded, err = SeedDefaultsIfMissing(path)
	require.NoError(t, err)
	assert.False(t, seeded)
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	if string(got) != defaultActionsYAML {
		t.Fatal("second seed altered default catalog bytes")
	}
}

func TestSeedDefaultsIfMissingNeverTouchesPresentFiles(t *testing.T) {
	for _, original := range []string{"", "version: nope\nactions: ["} {
		t.Run("present", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "actions.yml")
			require.NoError(t, os.WriteFile(path, []byte(original), 0o600))
			seeded, err := SeedDefaultsIfMissing(path)
			require.NoError(t, err)
			assert.False(t, seeded)
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, original, string(got))
		})
	}
}

func TestSeedDefaultsIfMissingExclusiveConcurrentInstallAndCleansTemps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "actions.yml")
	var wg sync.WaitGroup
	results := make(chan bool, 8)
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			seeded, err := SeedDefaultsIfMissing(path)
			results <- seeded
			errs <- err
		})
	}
	wg.Wait()
	close(results)
	close(errs)
	count := 0
	for seeded := range results {
		if seeded {
			count++
		}
	}
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, 1, count)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"actions.yml"}, func() []string {
		out := make([]string, len(entries))
		for i, entry := range entries {
			out[i] = entry.Name()
		}
		return out
	}())
}

func TestSeedDefaultsIfMissingReportsDirectoryFailure(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(parent, []byte("x"), 0o600))
	seeded, err := SeedDefaultsIfMissing(filepath.Join(parent, "actions.yml"))
	assert.False(t, seeded)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat actions seed target")
}
