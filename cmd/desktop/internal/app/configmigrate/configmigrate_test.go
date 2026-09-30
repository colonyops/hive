package configmigrate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// renameStep is a synthetic Set whose single real migration renames old_name
// to new_name, used to prove Apply actually transforms a below-current
// document rather than just bumping the version stamp.
func renameStep() Set {
	return Set{
		Name:                "widget",
		Baseline:            1,
		Current:             2,
		AllowMissingVersion: true,
		Migrations: []Migration{
			{To: 2, Migrate: func(doc map[string]any) error {
				if v, ok := doc["old_name"]; ok {
					doc["new_name"] = v
					delete(doc, "old_name")
				}
				return nil
			}},
		},
	}
}

func decodeDoc(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	return doc
}

func TestValidate_RegisteredSets(t *testing.T) {
	t.Parallel()

	sets := []Set{SettingsSet, FlowSet, ActionsSet, MCPLibrarySet, SkillLibrarySet, AgentWorkspaceSet}
	for _, s := range sets {
		t.Run(s.Name, func(t *testing.T) {
			t.Parallel()
			assert.NoError(t, s.Validate())
		})
	}
}

// The MCP cut-over (ADR mcp-replaces-the-agent-facing-http-api) retired the hive-http-api skill slug. A workspace
// still declaring it fails to open outright — resolveSkills refuses a slug no
// prompt id backs — so the rename has to happen before the manifest is loaded.
func TestAgentWorkspace_RenamesTheRetiredHTTPAPISkill(t *testing.T) {
	t.Parallel()

	raw := []byte("version: 1\nname: Hive\nagent: claude\nautonomy: ask\nskills:\n  - hive-flows\n  - hive-http-api\n  - hive-settings\n")

	migrated, changed, err := AgentWorkspaceSet.Apply(raw)
	require.NoError(t, err)
	require.True(t, changed)

	doc := decodeDoc(t, migrated)
	assert.Equal(t, AgentWorkspaceSet.Current, doc["version"])
	assert.Equal(t, []any{"hive-flows", "hive-mcp", "hive-settings"}, doc["skills"],
		"the slug is renamed in place, keeping its position")
}

// A manifest naming both would otherwise declare hive-mcp twice, which the
// workspace validator rejects as a duplicate — turning a migration into the
// breakage it exists to prevent.
func TestAgentWorkspace_SkillRenameDoesNotDuplicate(t *testing.T) {
	t.Parallel()

	raw := []byte("version: 1\nname: Hive\nagent: claude\nautonomy: ask\nskills:\n  - hive-http-api\n  - hive-mcp\n")

	migrated, changed, err := AgentWorkspaceSet.Apply(raw)
	require.NoError(t, err)
	require.True(t, changed)

	assert.Equal(t, []any{"hive-mcp"}, decodeDoc(t, migrated)["skills"])
}

// A manifest with no skills list at all is the common case, and must migrate
// to the new version without growing an empty key.
func TestAgentWorkspace_SkillRenameToleratesNoSkills(t *testing.T) {
	t.Parallel()

	migrated, changed, err := AgentWorkspaceSet.Apply([]byte("version: 1\nname: Hive\nagent: claude\nautonomy: ask\n"))
	require.NoError(t, err)
	require.True(t, changed)

	doc := decodeDoc(t, migrated)
	assert.Equal(t, AgentWorkspaceSet.Current, doc["version"])
	assert.NotContains(t, doc, "skills")
}

func TestValidate_GapChainRejected(t *testing.T) {
	t.Parallel()

	s := Set{
		Name:       "gappy",
		Baseline:   1,
		Current:    3,
		Migrations: []Migration{{To: 3}},
	}
	assert.Error(t, s.Validate())
}

func TestValidate_DuplicateStepRejected(t *testing.T) {
	t.Parallel()

	s := Set{
		Name:     "dupe",
		Baseline: 1,
		Current:  2,
		Migrations: []Migration{
			{To: 2},
			{To: 2},
		},
	}
	assert.Error(t, s.Validate())
}

func TestValidate_NoOpChainIsValid(t *testing.T) {
	t.Parallel()

	s := Set{Name: "flat", Baseline: 1, Current: 1}
	assert.NoError(t, s.Validate())
}

func TestApply_EmptyInput(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string][]byte{"nil": nil, "empty": {}, "whitespace": []byte("   \n")} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			migrated, changed, err := SettingsSet.Apply(raw)
			require.NoError(t, err)
			assert.Nil(t, migrated)
			assert.False(t, changed)
		})
	}
}

func TestApply_CorruptInputFailsDecodeNotPanic(t *testing.T) {
	t.Parallel()

	assert.NotPanics(t, func() {
		migrated, changed, err := SettingsSet.Apply([]byte("foo: [unterminated"))
		assert.Error(t, err)
		assert.False(t, changed)
		assert.Nil(t, migrated)
	})
}

func TestApply_VersionAboveCurrentRejected(t *testing.T) {
	t.Parallel()

	migrated, changed, err := renameStep().Apply([]byte("version: 3\n"))
	require.ErrorIs(t, err, ErrVersionTooNew)
	assert.False(t, changed)
	assert.Nil(t, migrated)
}

func TestApply_VersionBelowBaselineRejected(t *testing.T) {
	t.Parallel()

	_, _, err := renameStep().Apply([]byte("version: 0\n"))
	assert.Error(t, err)
}

func TestApply_NonIntegerVersionRejected(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string][]byte{
		"string": []byte("version: \"two\"\n"),
		"float":  []byte("version: 1.5\n"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			migrated, changed, err := renameStep().Apply(raw)
			require.Error(t, err)
			require.NotErrorIs(t, err, ErrVersionTooNew)
			assert.False(t, changed)
			assert.Nil(t, migrated)
		})
	}
}

func TestApply_VersionEqualCurrentIsNoOp(t *testing.T) {
	t.Parallel()

	s := renameStep()
	raw := []byte("version: 2\nnew_name: foo\n")
	migrated, changed, err := s.Apply(raw)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, raw, migrated)
}

func TestApply_AbsentVersionDefaultsToBaseline(t *testing.T) {
	t.Parallel()

	// Baseline == Current: an absent version defaults to Baseline, which is
	// already current, so this is a no-op (the Current == Baseline no-op
	// invariant every registered Set relies on before its first real step).
	s := Set{Name: "flat", Baseline: 1, Current: 1, AllowMissingVersion: true}
	raw := []byte("some_key: value\n")
	migrated, changed, err := s.Apply(raw)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, raw, migrated)
}

func TestApply_AbsentVersionRejectedWhenRequired(t *testing.T) {
	t.Parallel()

	migrated, changed, err := renameStep().Apply([]byte("old_name: hello\n"))
	require.NoError(t, err)
	require.True(t, changed)
	require.NotNil(t, migrated)

	required := renameStep()
	required.AllowMissingVersion = false
	migrated, changed, err = required.Apply([]byte("old_name: hello\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version field is required")
	assert.False(t, changed)
	assert.Nil(t, migrated)
}

func TestApply_MultipleDocumentsRejected(t *testing.T) {
	t.Parallel()

	migrated, changed, err := renameStep().Apply([]byte("old_name: hello\n---\nother: value\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple YAML documents")
	assert.False(t, changed)
	assert.Nil(t, migrated)
}

func TestApply_StepErrorAbortsWithNoBytes(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	s := Set{
		Name:                "broken",
		Baseline:            1,
		Current:             2,
		AllowMissingVersion: true,
		Migrations: []Migration{
			{To: 2, Migrate: func(doc map[string]any) error { return boom }},
		},
	}

	migrated, changed, err := s.Apply([]byte("k: v\n"))
	require.ErrorIs(t, err, boom)
	assert.False(t, changed)
	assert.Nil(t, migrated)
}

func TestApply_RenameStepTransformsAndRestampsVersion(t *testing.T) {
	t.Parallel()

	s := renameStep()
	migrated, changed, err := s.Apply([]byte("old_name: hello\n"))
	require.NoError(t, err)
	assert.True(t, changed)
	require.NotNil(t, migrated)

	doc := decodeDoc(t, migrated)
	assert.Equal(t, 2, doc["version"])
	assert.Equal(t, "hello", doc["new_name"])
	_, hasOldName := doc["old_name"]
	assert.False(t, hasOldName, "old_name should be removed by the rename step")
}

func TestApply_IsIdempotent(t *testing.T) {
	t.Parallel()

	s := renameStep()
	migrated, changed, err := s.Apply([]byte("old_name: hello\n"))
	require.NoError(t, err)
	require.True(t, changed)

	_, changedAgain, err := s.Apply(migrated)
	require.NoError(t, err)
	assert.False(t, changedAgain, "re-applying to an already-migrated document must be a no-op")
}

// The settings decoder is strict, so the retired installer's `skills` section
// (ADR skills-are-declared-by-a-workspace) has to be dropped before decode or
// every user who ever opened Settings ▸ Skills fails startup on upgrade.
func TestSettings_DropsTheRetiredSkillsSection(t *testing.T) {
	t.Parallel()

	raw := []byte("version: 2\npolling:\n  interval: 5m\nskills:\n  auto_update: true\n  targets:\n    claude: {dir: ~/.claude/skills}\n")

	migrated, changed, err := SettingsSet.Apply(raw)
	require.NoError(t, err)
	require.True(t, changed)

	doc := decodeDoc(t, migrated)
	assert.Equal(t, SettingsSet.Current, doc["version"])
	assert.NotContains(t, doc, "skills")
	assert.Contains(t, doc, "polling", "an unrelated section is untouched")
}

// The status bar ships on, but a saved settings.yaml carries an explicit
// `terminal_show_status_bar: false` from when off was the default, so the flip
// only reaches existing files through a one-time migration.
func TestSettings_TurnsTheTerminalStatusBarOn(t *testing.T) {
	t.Parallel()

	raw := []byte("version: 3\nappearance:\n  theme: midnight\n  terminal_show_windows: false\n  terminal_show_status_bar: false\n")

	migrated, changed, err := SettingsSet.Apply(raw)
	require.NoError(t, err)
	require.True(t, changed)

	doc := decodeDoc(t, migrated)
	assert.Equal(t, SettingsSet.Current, doc["version"])
	appearance, ok := doc["appearance"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, appearance["terminal_show_status_bar"])
	assert.Equal(t, false, appearance["terminal_show_windows"], "an unrelated appearance value is untouched")
	assert.Equal(t, "midnight", appearance["theme"])
}

func TestSettings_DropsDevelopmentInstanceID(t *testing.T) {
	t.Parallel()

	raw := []byte("version: 4\ndevelopment:\n  instance:\n    id: worktree-a\n  github:\n    api_base: http://127.0.0.1:7777\n")

	migrated, changed, err := SettingsSet.Apply(raw)
	require.NoError(t, err)
	require.True(t, changed)

	doc := decodeDoc(t, migrated)
	assert.Equal(t, SettingsSet.Current, doc["version"])
	development, ok := doc["development"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, development, "instance")
	assert.Contains(t, development, "github", "an unrelated development value is untouched")
}

func TestSettings_TurnsTheTerminalFontSizeIntoPixels(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value string
		want  any
	}{
		{"a name", "large", 14},
		{"the largest name", "xxl", 18},
		{"a quoted number", `"15"`, 15},
		{"an integer", "20", 20},
		{"empty", `""`, nil},
		{"unreadable", "enormous", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw := []byte("version: 5\nappearance:\n  theme: midnight\n  terminal_font_size: " + tc.value + "\n")
			migrated, changed, err := SettingsSet.Apply(raw)
			require.NoError(t, err)
			require.True(t, changed)

			doc := decodeDoc(t, migrated)
			assert.Equal(t, SettingsSet.Current, doc["version"])
			appearance, ok := doc["appearance"].(map[string]any)
			require.True(t, ok)
			if tc.want == nil {
				assert.NotContains(t, appearance, "terminal_font_size")
			} else {
				assert.Equal(t, tc.want, appearance["terminal_font_size"])
			}
			assert.Equal(t, "midnight", appearance["theme"], "an unrelated appearance value is untouched")
		})
	}
}

// A file with no appearance section decodes onto the defaults, which say on,
// so the step has nothing to add and must not invent the section.
func TestSettings_TerminalStatusBarStepLeavesAMissingAppearanceSectionAlone(t *testing.T) {
	t.Parallel()

	migrated, changed, err := SettingsSet.Apply([]byte("version: 3\npolling:\n  interval: 5m\n"))
	require.NoError(t, err)
	require.True(t, changed)

	doc := decodeDoc(t, migrated)
	assert.Equal(t, SettingsSet.Current, doc["version"])
	assert.NotContains(t, doc, "appearance")
}

// The identity case #309 is about: a manifest listing exactly the shipped set
// resolves to the same skills through the hive package, so rewriting it is
// not inference. It is also the case that actually bites — the seeded hive
// workspace every install carries.
func TestAgentWorkspace_CollapsesTheExactShippedSetOntoTheHivePackage(t *testing.T) {
	t.Parallel()

	raw := []byte("version: 2\nname: Hive\nagent: claude\nautonomy: ask\nskills:\n" +
		"  - hive-actions\n  - hive-agent-workspaces\n  - hive-flows\n  - hive-mcp\n  - hive-settings\n  - hive-webhook-sources\n")

	migrated, changed, err := AgentWorkspaceSet.Apply(raw)
	require.NoError(t, err)
	require.True(t, changed)

	assert.Equal(t, []any{"hive"}, decodeDoc(t, migrated)["skills"])
}

// A partial list cannot be migrated: mapping [hive-mcp] to [hive] would grant
// five skills the workspace never carried. It is left for a human, which is
// what the editor's warning now points at (#307).
func TestAgentWorkspace_LeavesAPartialSkillListAlone(t *testing.T) {
	t.Parallel()

	for name, skills := range map[string][]string{
		"subset":         {"hive-mcp"},
		"missing one":    {"hive-actions", "hive-agent-workspaces", "hive-flows", "hive-mcp", "hive-settings"},
		"one unexpected": {"hive-actions", "hive-agent-workspaces", "hive-flows", "hive-mcp", "hive-settings", "terraform-plan"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw := []byte("version: 2\nname: Hive\nagent: claude\nautonomy: ask\nskills:\n")
			for _, slug := range skills {
				raw = append(raw, ("  - " + slug + "\n")...)
			}

			migrated, _, err := AgentWorkspaceSet.Apply(raw)
			require.NoError(t, err)

			want := make([]any, 0, len(skills))
			for _, slug := range skills {
				want = append(want, slug)
			}
			assert.Equal(t, want, decodeDoc(t, migrated)["skills"])
		})
	}
}

// TestAgentWorkspace_TurnsEveryPostureIntoACommand is the whole cross-product,
// not a sample: a posture that silently produced no command would leave a
// workspace unable to launch after an upgrade, which is worse than the
// refusal this schema change removed.
func TestAgentWorkspace_TurnsEveryPostureIntoACommand(t *testing.T) {
	t.Parallel()

	want := map[string]map[string]string{
		"claude": {
			"ask":  "claude" + autonomyCommandTail,
			"auto": "claude --permission-mode acceptEdits" + autonomyCommandTail,
			"full": "claude --dangerously-skip-permissions" + autonomyCommandTail,
		},
		"codex": {
			"ask":  "codex",
			"auto": "codex --ask-for-approval on-request --sandbox workspace-write",
			"full": "codex --dangerously-bypass-approvals-and-sandbox",
		},
	}

	for agent, postures := range want {
		for posture, command := range postures {
			t.Run(agent+"/"+posture, func(t *testing.T) {
				t.Parallel()

				raw := []byte("version: 3\nname: Demo\nagent: " + agent + "\nautonomy: " + posture + "\n")
				migrated, changed, err := AgentWorkspaceSet.Apply(raw)
				require.NoError(t, err)
				require.True(t, changed)

				doc := decodeDoc(t, migrated)
				assert.Equal(t, command, doc["command"])
				assert.NotContains(t, doc, "autonomy", "the posture key is retired, not left beside its replacement")
			})
		}
	}
}

// An omitted autonomy meant "ask" at version 3, so it must keep meaning that
// through the migration rather than producing a bare command with no wiring.
func TestAgentWorkspace_AnOmittedPostureMigratesAsAsk(t *testing.T) {
	t.Parallel()

	migrated, _, err := AgentWorkspaceSet.Apply([]byte("version: 3\nname: Demo\nagent: claude\n"))
	require.NoError(t, err)
	assert.Equal(t, "claude"+autonomyCommandTail, decodeDoc(t, migrated)["command"])
}

// The agent that motivated the change: "pi" had no launch mapping, so no
// posture ever resolved for it. It migrates to its own bare name — a command
// the user can now edit, where before there was nothing to edit.
func TestAgentWorkspace_AnUnmappedAgentMigratesToItsOwnName(t *testing.T) {
	t.Parallel()

	migrated, _, err := AgentWorkspaceSet.Apply([]byte("version: 3\nname: Demo\nagent: pi\nautonomy: full\n"))
	require.NoError(t, err)

	doc := decodeDoc(t, migrated)
	assert.Equal(t, "pi", doc["command"])
	assert.NotContains(t, doc, "autonomy")
}

// A manifest hand-authored against the new schema keeps its command: the user
// wrote the newer field on purpose, and autonomy beside it is the stale half.
func TestAgentWorkspace_KeepsAHandWrittenCommand(t *testing.T) {
	t.Parallel()

	raw := []byte("version: 3\nname: Demo\nagent: claude\nautonomy: full\ncommand: pi --custom\n")
	migrated, _, err := AgentWorkspaceSet.Apply(raw)
	require.NoError(t, err)

	doc := decodeDoc(t, migrated)
	assert.Equal(t, "pi --custom", doc["command"])
	assert.NotContains(t, doc, "autonomy")
}

func TestAgentWorkspace_DropsTheAgentLabel(t *testing.T) {
	t.Parallel()

	migrated, changed, err := AgentWorkspaceSet.Apply([]byte("version: 4\nname: Demo\nagent: claude\ncommand: claude --model opus\n"))
	require.NoError(t, err)
	require.True(t, changed)

	doc := decodeDoc(t, migrated)
	assert.NotContains(t, doc, "agent")
	assert.Equal(t, "claude --model opus", doc["command"], "the command it derives from is untouched")
}

// Version 4 let a manifest omit command: and backfill it from the agent, so
// the step has to write the command out before it deletes the agent.
func TestAgentWorkspace_MaterializesACommandBeforeDroppingTheAgent(t *testing.T) {
	t.Parallel()

	for agent, want := range map[string]string{
		"claude": "claude" + autonomyCommandTail,
		"codex":  "codex",
		"pi":     "pi",
	} {
		t.Run(agent, func(t *testing.T) {
			t.Parallel()

			migrated, _, err := AgentWorkspaceSet.Apply([]byte("version: 4\nname: Demo\nagent: " + agent + "\n"))
			require.NoError(t, err)

			doc := decodeDoc(t, migrated)
			assert.Equal(t, want, doc["command"])
			assert.NotContains(t, doc, "agent")
		})
	}
}

func TestAgentWorkspace_MigratesFromTheBaselineToACommandAlone(t *testing.T) {
	t.Parallel()

	migrated, _, err := AgentWorkspaceSet.Apply([]byte("version: 1\nname: Demo\nagent: claude\nautonomy: full\n"))
	require.NoError(t, err)

	doc := decodeDoc(t, migrated)
	assert.Equal(t, AgentWorkspaceSet.Current, doc["version"])
	assert.Equal(t, "claude --dangerously-skip-permissions"+autonomyCommandTail, doc["command"])
	assert.NotContains(t, doc, "agent")
	assert.NotContains(t, doc, "autonomy")
}
