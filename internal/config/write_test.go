package config_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
)

func write(t *testing.T, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	return path
}

func workspaceDir(t *testing.T, repos int) string {
	t.Helper()
	dir := t.TempDir()
	for i := range repos {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, fmt.Sprintf("repo-%d", i), ".git"), 0o755))
	}
	return dir
}

// Clear HIVE_DEFAULT_AGENT because Hive applies it before validation; ambient
// shell state must not decide whether these tests pass.
func loadHive(t *testing.T, path string) *config.Config {
	t.Helper()
	t.Setenv("HIVE_DEFAULT_AGENT", "")
	loaded, err := config.Load(path, t.TempDir())
	require.NoError(t, err)
	return loaded
}

func apply(t *testing.T, path string, edit config.Edit) error {
	t.Helper()
	t.Setenv("HIVE_DEFAULT_AGENT", "")
	dataDir := t.TempDir()
	return config.ApplyEdit(path, edit, func(candidate string) error {
		_, err := config.Load(candidate, dataDir)
		return err
	})
}

func TestCheckWorkspaceRejectsARepositoryItself(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))

	require.ErrorIs(t, config.CheckWorkspace(dir), config.ErrWorkspaceIsRepo)
	require.Error(t, config.CheckWorkspace(filepath.Join(dir, "missing")))
	require.NoError(t, config.CheckWorkspace(t.TempDir()))
}

// Hive rejects the whole file when agents.default names no profile.
func TestApplyWritesAConfigHiveCanLoad(t *testing.T) {
	repos := workspaceDir(t, 1)
	path := filepath.Join(t.TempDir(), "config.yaml")

	require.NoError(t, apply(t, path, config.Edit{
		DefaultAgent: "opencode",
		Profiles: []config.EditProfile{
			{Name: "opencode", Command: "opencode", Flags: []string{"--agent", "free-permissions-runner"}},
			{Name: "claude", Command: "claude"},
		},
		Workspaces: []string{repos},
	}))

	loaded := loadHive(t, path)
	assert.Equal(t, "opencode", loaded.Agents.Default)
	assert.Equal(t, []string{repos}, loaded.Workspaces)
	assert.Equal(t, []string{"--agent", "free-permissions-runner"}, loaded.Agents.Profiles["opencode"].Flags)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "# Hive configuration", "a created file explains itself")
}

// Pin the whole output because Contains cannot detect a moved key or misplaced
// comment. yaml.v3 does not preserve blank lines between keys.
func TestApplyKeepsEverythingItDoesNotOwn(t *testing.T) {
	repos := workspaceDir(t, 1)
	path := write(t, t.TempDir(), `# my hive setup
copy_command: pbcopy

# these matter to me
rules:
  - pattern: ""
    max_recycled: 3

agents:
  default: claude
  agent_selector: true
  claude:
    command: claude
    flags: []

workspaces:
  - /old/place
`)

	require.NoError(t, apply(t, path, config.Edit{
		DefaultAgent: "codex",
		Profiles:     []config.EditProfile{{Name: "codex", Command: "codex", Flags: []string{"--full-auto"}}},
		Workspaces:   []string{repos},
	}))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, `# my hive setup
copy_command: pbcopy
# these matter to me
rules:
  - pattern: ""
    max_recycled: 3
agents:
  default: codex
  agent_selector: true
  codex:
    command: codex
    flags: [--full-auto]
workspaces:
  - `+repos+`
`, string(raw))

	loaded := loadHive(t, path)
	assert.Equal(t, "codex", loaded.Agents.Default)
	assert.Len(t, loaded.Rules, 1)
}

// Settings > Hive CLI after first run is an edit of the file first run
// rendered, so the template's own comments have to survive an in-place edit.
func TestApplyEditsTheFileItRenderedWithoutLosingItsComments(t *testing.T) {
	repos := workspaceDir(t, 1)
	more := workspaceDir(t, 2)
	path := filepath.Join(t.TempDir(), "config.yaml")
	edit := config.Edit{
		DefaultAgent: "claude",
		Profiles:     []config.EditProfile{{Name: "claude"}},
		Workspaces:   []string{repos},
	}
	require.NoError(t, apply(t, path, edit))

	edit.Workspaces = []string{repos, more}
	require.NoError(t, apply(t, path, edit))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "# Hive configuration")
	assert.Contains(t, string(raw), "# Parent folders that hold your git repositories.")
	assert.Equal(t, []string{repos, more}, loadHive(t, path).Workspaces)
}

// "<<" is not a profile name this app invented, so reconciling it away would
// edit a line the user wrote for their own reasons. Hive does not resolve a
// merge key inside agents — its own UnmarshalYAML reads "<<" as a profile key —
// so preserving it is about
// not destroying someone's text, not about making the merge work. Load does
// not report it as a profile either, or a save of exactly what was loaded
// would replace the alias with a mapping.
func TestApplyKeepsAYAMLMergeKeyInsideAgents(t *testing.T) {
	repos := workspaceDir(t, 1)
	path := write(t, t.TempDir(), `shared: &shared
  codex:
    command: codex
    flags: []

agents:
  <<: *shared
  default: claude
  claude:
    command: claude
    flags: []
`)

	require.NoError(t, apply(t, path, config.Edit{
		DefaultAgent: "claude",
		Profiles:     []config.EditProfile{{Name: "claude", Command: "claude", Flags: []string{}}},
		Workspaces:   []string{repos},
	}))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "<<: *shared")

	loaded := loadHive(t, path)
	assert.Equal(t, "claude", loaded.Agents.Default, "the file still loads, which is the contract that matters")
}

// An anchor on a node the edit replaces has to move to the replacement, or
// the alias that referenced it turns a loadable file into an unreadable one.
func TestApplyKeepsAnAnchorAnAliasStillNeeds(t *testing.T) {
	repos := workspaceDir(t, 1)
	path := write(t, t.TempDir(), `workspaces: &ws
  - /old/place
mirror: *ws
agents:
  default: claude
  claude: {}
`)

	require.NoError(t, apply(t, path, config.Edit{
		DefaultAgent: "claude",
		Profiles:     []config.EditProfile{{Name: "claude"}},
		Workspaces:   []string{repos},
	}))

	assert.Equal(t, []string{repos}, loadHive(t, path).Workspaces)
}

func TestApplyPreservesAnUnknownKeyInsideAProfileItRewrites(t *testing.T) {
	repos := workspaceDir(t, 1)
	path := write(t, t.TempDir(), `agents:
  default: claude
  claude:
    command: claude
    flags: []
    future_hive_key: keep-me
`)

	require.NoError(t, apply(t, path, config.Edit{
		DefaultAgent: "claude",
		Profiles:     []config.EditProfile{{Name: "claude", Command: "claude", Flags: []string{"--dangerously-skip-permissions"}}},
		Workspaces:   []string{repos},
	}))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "future_hive_key: keep-me")
	assert.Contains(t, string(raw), "--dangerously-skip-permissions")
}

// Hive reads repo_dirs when workspaces is empty, so leaving it would make an
// older Hive read stale data instead of the edit.
func TestApplyDropsTheDeprecatedRepoDirsKeyItReplaces(t *testing.T) {
	repos := workspaceDir(t, 1)
	path := write(t, t.TempDir(), "repo_dirs:\n  - /old\nagents:\n  default: claude\n  claude: {}\n")

	require.NoError(t, apply(t, path, config.Edit{
		DefaultAgent: "claude",
		Profiles:     []config.EditProfile{{Name: "claude"}},
		Workspaces:   []string{repos},
	}))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "repo_dirs")
	assert.Contains(t, string(raw), repos)
}

// A file with comments but no keys is rendered whole because yaml.v3 attaches
// a keyless document's comments to no node, so an in-place edit would drop
// them and
// leave a config with no header at all.
func TestApplyFillsAFileHiveCreatedButLeftEmpty(t *testing.T) {
	repos := workspaceDir(t, 1)
	for name, existing := range map[string]string{
		"comments only": "# Hive configuration\n",
		"empty":         "",
	} {
		t.Run(name, func(t *testing.T) {
			path := write(t, t.TempDir(), existing)

			require.NoError(t, apply(t, path, config.Edit{
				DefaultAgent: "claude",
				Profiles:     []config.EditProfile{{Name: "claude"}},
				Workspaces:   []string{repos},
			}))

			loaded := loadHive(t, path)
			assert.Equal(t, []string{repos}, loaded.Workspaces)

			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(raw), "Reference: https://hive.colonyops.io/configuration")
		})
	}
}

func TestApplyRefusesAnEditThatWouldNotLoadAndLeavesTheFileAlone(t *testing.T) {
	repos := workspaceDir(t, 1)
	original := "agents:\n  default: claude\n  claude: {}\n"
	path := write(t, t.TempDir(), original)

	for name, edit := range map[string]config.Edit{
		"default names no chosen profile": {
			DefaultAgent: "codex",
			Profiles:     []config.EditProfile{{Name: "claude"}},
			Workspaces:   []string{repos},
		},
		"no agents at all": {
			DefaultAgent: "claude",
			Workspaces:   []string{repos},
		},
		"an agent with no name": {
			DefaultAgent: "claude",
			Profiles:     []config.EditProfile{{Name: "claude"}, {Name: "  "}},
			Workspaces:   []string{repos},
		},
		"a hive setting as an agent name": {
			DefaultAgent: "claude",
			Profiles:     []config.EditProfile{{Name: "claude"}, {Name: "agent_selector"}},
			Workspaces:   []string{repos},
		},
		"a merge key as an agent name": {
			DefaultAgent: "claude",
			Profiles:     []config.EditProfile{{Name: "claude"}, {Name: "<<"}},
			Workspaces:   []string{repos},
		},
		"no workspaces": {
			DefaultAgent: "claude",
			Profiles:     []config.EditProfile{{Name: "claude"}},
		},
		"a workspace that is not there": {
			DefaultAgent: "claude",
			Profiles:     []config.EditProfile{{Name: "claude"}},
			Workspaces:   []string{filepath.Join(repos, "nope")},
		},
		"the same agent twice": {
			DefaultAgent: "claude",
			Profiles:     []config.EditProfile{{Name: "claude"}, {Name: "claude"}},
			Workspaces:   []string{repos},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := apply(t, path, edit)
			require.Error(t, err)
			require.ErrorAs(t, err, new(config.InvalidEditError), "the user can fix this one")
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, original, string(raw), "a rejected edit writes nothing")
		})
	}
}

// Hive validates rules[].agent against the profile map exactly as it does
// agents.default, and this package owns neither the rules nor a way to repair
// them — so an edit that would strand one is refused, not written.
func TestApplyRefusesToDropAProfileARuleUses(t *testing.T) {
	repos := workspaceDir(t, 1)
	original := `rules:
  - pattern: acme/*
    agent: claude
agents:
  default: claude
  claude: {}
  codex: {}
`
	path := write(t, t.TempDir(), original)

	err := apply(t, path, config.Edit{
		DefaultAgent: "codex",
		Profiles:     []config.EditProfile{{Name: "codex"}},
		Workspaces:   []string{repos},
	})

	require.Error(t, err)
	require.ErrorAs(t, err, new(config.InvalidEditError))
	assert.Contains(t, err.Error(), `"acme/*"`)
	assert.Contains(t, err.Error(), `"claude"`)
	raw, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, original, string(raw))
}

func TestApplyLetsTheCheckRefuseTheCandidateBeforeItLands(t *testing.T) {
	repos := workspaceDir(t, 1)
	original := "agents:\n  default: claude\n  claude: {}\n"
	path := write(t, t.TempDir(), original)

	var candidate string
	err := config.ApplyEdit(path, config.Edit{
		DefaultAgent: "claude",
		Profiles:     []config.EditProfile{{Name: "claude"}},
		Workspaces:   []string{repos},
	}, func(p string) error {
		raw, readErr := os.ReadFile(p)
		require.NoError(t, readErr)
		candidate = string(raw)
		return errors.New("hive says no")
	})

	require.Error(t, err)
	require.ErrorAs(t, err, new(config.InvalidEditError))
	assert.Contains(t, err.Error(), "hive says no")
	assert.Contains(t, candidate, repos, "the check saw the edit it was asked about")
	raw, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, original, string(raw))
	entries, readErr := os.ReadDir(filepath.Dir(path))
	require.NoError(t, readErr)
	assert.Len(t, entries, 1, "no temp file is left behind")
}

func TestApplyRefusesAFileWhoseRootIsNotAMapping(t *testing.T) {
	repos := workspaceDir(t, 1)
	original := "- not\n- a\n- config\n"
	path := write(t, t.TempDir(), original)

	require.Error(t, apply(t, path, config.Edit{
		DefaultAgent: "claude",
		Profiles:     []config.EditProfile{{Name: "claude"}},
		Workspaces:   []string{repos},
	}))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, original, string(raw))
}

// Hive's profile lookup is exact, so the edit that is validated has to be the
// edit that is written: a padded default that validates trimmed and lands
// padded fails the next load. Two spellings of one folder would have hive scan
// it twice.
func TestApplyNormalizesTheEditItWrites(t *testing.T) {
	repos := workspaceDir(t, 1)
	path := filepath.Join(t.TempDir(), "config.yaml")

	require.NoError(t, apply(t, path, config.Edit{
		DefaultAgent: " claude ",
		Profiles:     []config.EditProfile{{Name: "claude "}, {Name: " codex", Command: " codex "}},
		Workspaces:   []string{" " + repos + " ", repos + "/", "", repos},
	}))

	loaded := loadHive(t, path)
	assert.Equal(t, "claude", loaded.Agents.Default)
	assert.Equal(t, "codex", loaded.Agents.Profiles["codex"].Command)
	assert.Equal(t, []string{repos}, loaded.Workspaces)
}

func TestApplyKeepsASymlinkedConfigASymlink(t *testing.T) {
	repos := workspaceDir(t, 1)
	dotfiles := t.TempDir()
	target := write(t, dotfiles, "agents:\n  default: claude\n  claude: {}\n")
	link := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.Symlink(target, link))

	require.NoError(t, apply(t, link, config.Edit{
		DefaultAgent: "claude",
		Profiles:     []config.EditProfile{{Name: "claude"}},
		Workspaces:   []string{repos},
	}))

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the dotfiles link is still a link")
	raw, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Contains(t, string(raw), repos, "and the dotfiles repository holds the edit")
	entries, err := os.ReadDir(dotfiles)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temp file is left beside the target")
}

func TestApplyQuotesAPathThatWouldChangeMeaningUnquoted(t *testing.T) {
	dir := t.TempDir()
	odd := filepath.Join(dir, "code: mine")
	require.NoError(t, os.MkdirAll(odd, 0o755))
	path := filepath.Join(dir, "config.yaml")

	require.NoError(t, apply(t, path, config.Edit{
		DefaultAgent: "claude",
		Profiles:     []config.EditProfile{{Name: "claude"}},
		Workspaces:   []string{odd},
	}))

	loaded := loadHive(t, path)
	assert.Equal(t, []string{odd}, loaded.Workspaces)
}

// O_EXCL preserves a file created between the settings read and the click.
func TestCreateWritesTheHeaderOnceAndLeavesAnExistingFileAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")

	require.NoError(t, config.CreateFile(path))
	created, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(created), "# Hive configuration"))
	loadHive(t, path)

	require.NoError(t, os.WriteFile(path, []byte("agents:\n  default: pi\n"), 0o644))
	require.NoError(t, config.CreateFile(path))
	kept, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "agents:\n  default: pi\n", string(kept))
}

func TestYAMLFlowSequence(t *testing.T) {
	assert.Equal(t, "[]", config.YAMLFlowSequence(nil))
	assert.Equal(t, "[]", config.YAMLFlowSequence([]string{}))
	assert.Equal(t, "[--foo]", config.YAMLFlowSequence([]string{"--foo"}))
	assert.Equal(t, "[--foo, --bar]", config.YAMLFlowSequence([]string{"--foo", "--bar"}))
}

func TestYAMLScalarQuotesWhatWouldChangeMeaning(t *testing.T) {
	assert.Equal(t, "plain", config.YAMLScalar("plain"))
	assert.Equal(t, `'a: b'`, config.YAMLScalar("a: b"))
	assert.Equal(t, `"yes"`, config.YAMLScalar("yes"))
}
