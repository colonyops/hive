package agentws

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/schedule"
)

func manifestRoot(t *testing.T, body string) string {
	t.Helper()

	root := t.TempDir()
	dir := filepath.Join(root, "product")
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, manifestFileName), []byte(body), 0o600))
	return root
}

func loadManifest(t *testing.T, root string) (Workspace, string) {
	t.Helper()

	path := filepath.Join(root, "product", manifestFileName)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	w, err := LoadWorkspace(path)
	require.NoError(t, err)
	return w, string(raw)
}

const baseManifest = "version: 5\nname: Product\ncommand: " + promptedCommand + "\n"

// writeSchedules saves specs the way the editor does: as part of a whole
// manifest edit through WriteManifest.
func writeSchedules(t *testing.T, root string, specs ...schedule.Spec) {
	t.Helper()
	require.NoError(t, WriteManifest(root, "product", ManifestEdit{
		Name: "Product", Command: promptedCommand, Schedules: specs,
	}))
}

// WriteSchedules is the per-list write: the schedules: sequence says exactly
// what it is handed, and every other key, comment included, is untouched.
func TestWriteSchedulesTouchesOnlyTheSchedulesList(t *testing.T) {
	t.Parallel()

	root := manifestRoot(t, "# keep me\n"+baseManifest+"mcps: [playwright] # and me\n")

	require.NoError(t, WriteSchedules(root, "product", []schedule.Spec{
		{ID: "weekly", Name: "Weekly summary", Cron: "0 9 * * 5", Prompt: "Summarize the week."},
	}))
	w, raw := loadManifest(t, root)
	require.Len(t, w.Schedules, 1)
	assert.Equal(t, "weekly", w.Schedules[0].ID)
	assert.Equal(t, []string{"playwright"}, w.MCPs)
	assert.Contains(t, raw, "# keep me")
	assert.Contains(t, raw, "# and me")

	err := WriteSchedules(root, "no-such-workspace", nil)
	require.Error(t, err, "a schedule has nowhere to live without a manifest")
}

func TestWriteManifestAddsSchedules(t *testing.T) {
	t.Parallel()

	root := manifestRoot(t, baseManifest)

	writeSchedules(t, root,
		schedule.Spec{ID: "weekly", Name: "Weekly summary", Cron: "0 9 * * 5", Prompt: "Summarize the week."},
		schedule.Spec{ID: "daily", Cron: "@daily", Prompt: "Standup."},
	)

	w, raw := loadManifest(t, root)
	require.Len(t, w.Schedules, 2)
	assert.Equal(t, "weekly", w.Schedules[0].ID)
	assert.Equal(t, "Weekly summary", w.Schedules[0].Name)
	assert.Equal(t, "0 9 * * 5", w.Schedules[0].Cron)
	assert.Equal(t, "Summarize the week.", w.Schedules[0].Prompt)
	assert.Equal(t, "daily", w.Schedules[1].ID)
	assert.Equal(t, "product", w.Schedules[1].Workspace)
	assert.Empty(t, w.Schedules[1].Name, "a schedule with no name round trips without one; DisplayName falls back to the id")
	assert.NotContains(t, raw, "name: daily", "the id is not written back as a name the user never typed")
}

// TestWriteManifestRemovesTheDefaultScheduleKeys: name, disabled and on_missed
// are written only when they say something, so a schedule returned to its
// defaults leaves no leftovers claiming otherwise.
func TestWriteManifestRemovesTheDefaultScheduleKeys(t *testing.T) {
	t.Parallel()

	root := manifestRoot(t, baseManifest)
	spec := schedule.Spec{
		ID: "weekly", Name: "Weekly summary", Cron: "@daily", Prompt: "go",
		Disabled: true, OnMissed: schedule.OnMissedSkip,
	}
	writeSchedules(t, root, spec)

	_, raw := loadManifest(t, root)
	assert.Contains(t, raw, "name: Weekly summary")
	assert.Contains(t, raw, "disabled: true")
	assert.Contains(t, raw, "on_missed: skip")

	spec.Name = ""
	spec.Disabled = false
	spec.OnMissed = schedule.OnMissedRun
	writeSchedules(t, root, spec)

	w, raw := loadManifest(t, root)
	require.Len(t, w.Schedules, 1, "an entry the list still names is updated rather than appended beside itself")
	assert.NotContains(t, raw, "Weekly summary", "the name key goes with the name; the manifest's own name: stays")
	assert.NotContains(t, raw, "disabled")
	assert.NotContains(t, raw, "on_missed")
	assert.Empty(t, w.Schedules[0].Name)
	assert.Equal(t, "weekly", w.Schedules[0].DisplayName(), "a nameless schedule still shows as its id")
	assert.False(t, w.Schedules[0].Disabled)
	assert.Empty(t, w.Schedules[0].OnMissed)
}

func TestWriteManifestUsesALiteralBlockForAMultiLinePrompt(t *testing.T) {
	t.Parallel()

	root := manifestRoot(t, baseManifest)
	prompt := "Summarize product activity.\n\nList every open question at the end.\n"
	writeSchedules(t, root, schedule.Spec{ID: "weekly", Cron: "@daily", Prompt: prompt})

	w, raw := loadManifest(t, root)
	assert.Contains(t, raw, "prompt: |")
	assert.Contains(t, raw, "List every open question at the end.")
	assert.Equal(t, prompt, w.Schedules[0].Prompt, "the block round trips byte for byte")
}

// TestWriteManifestKeepsScheduleComments is the whole reason this is a
// node-tree edit: a hand-authored manifest survives an edit made from the UI.
func TestWriteManifestKeepsScheduleComments(t *testing.T) {
	t.Parallel()

	original := `# hand-authored: do not lose me
version: 5
name: Product
# the agent that runs here
command: ` + promptedCommand + `
schedules:
  # the one that matters
  - id: weekly
    name: Weekly summary
    cron: "0 9 * * 5"
    prompt: Summarize the week.
  - id: daily
    cron: "@daily"
    prompt: Standup.
`
	root := manifestRoot(t, original)

	writeSchedules(t, root,
		schedule.Spec{ID: "weekly", Name: "Weekly summary", Cron: "0 9 * * 5", Prompt: "Summarize the week."},
		schedule.Spec{ID: "daily", Name: "Daily standup", Cron: "0 9 * * 1-5", Prompt: "Standup, please."},
	)

	w, raw := loadManifest(t, root)
	assert.Contains(t, raw, "# hand-authored: do not lose me")
	assert.Contains(t, raw, "# the agent that runs here")
	assert.Contains(t, raw, "# the one that matters")

	require.Len(t, w.Schedules, 2)
	assert.Equal(t, "Weekly summary", w.Schedules[0].Name)
	assert.Equal(t, "0 9 * * 5", w.Schedules[0].Cron)
	assert.Equal(t, "Daily standup", w.Schedules[1].Name)
	assert.Equal(t, "0 9 * * 1-5", w.Schedules[1].Cron)
	assert.Equal(t, "Standup, please.", w.Schedules[1].Prompt)
}

// A write reconciles the sequence to exactly the list it is handed, in the
// order it is handed: an entry the list stops naming is gone, and an empty
// list takes the key with it.
func TestWriteManifestRemovesSchedulesTheListNoLongerNames(t *testing.T) {
	t.Parallel()

	root := manifestRoot(t, baseManifest)
	writeSchedules(t, root,
		schedule.Spec{ID: "weekly", Cron: "@weekly", Prompt: "go"},
		schedule.Spec{ID: "daily", Cron: "@daily", Prompt: "go"},
	)

	writeSchedules(t, root,
		schedule.Spec{ID: "daily", Cron: "@daily", Prompt: "go"},
		schedule.Spec{ID: "weekly", Cron: "@weekly", Prompt: "go"},
	)
	w, _ := loadManifest(t, root)
	require.Len(t, w.Schedules, 2)
	assert.Equal(t, "daily", w.Schedules[0].ID, "the editor owns the order the file lists them in")
	assert.Equal(t, "weekly", w.Schedules[1].ID)

	writeSchedules(t, root, schedule.Spec{ID: "daily", Cron: "@daily", Prompt: "go"})
	w, _ = loadManifest(t, root)
	require.Len(t, w.Schedules, 1)
	assert.Equal(t, "daily", w.Schedules[0].ID)

	writeSchedules(t, root)
	w, raw := loadManifest(t, root)
	assert.Empty(t, w.Schedules)
	assert.NotContains(t, raw, "schedules", "an empty list takes the key with it rather than leaving schedules: []")
}

// TestWriteManifestReplacesAnEmptySchedulesKey: `schedules:` with nothing
// under it parses as null rather than as a list, so the writer has to replace
// the value instead of appending to it.
func TestWriteManifestReplacesAnEmptySchedulesKey(t *testing.T) {
	t.Parallel()

	root := manifestRoot(t, baseManifest+"schedules:\n")
	writeSchedules(t, root, schedule.Spec{ID: "weekly", Cron: "@daily", Prompt: "go"})

	w, _ := loadManifest(t, root)
	require.Len(t, w.Schedules, 1)
	assert.Equal(t, "weekly", w.Schedules[0].ID)
}
