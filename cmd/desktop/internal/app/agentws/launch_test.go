package agentws

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveRendersTheCommandTemplate covers the whole data contract in one
// line, because a field silently missing from LaunchData renders as an empty
// word rather than an error the caller would notice.
func TestResolveRendersTheCommandTemplate(t *testing.T) {
	t.Parallel()

	w := Workspace{
		Command: `claude --mcp-config {{ .MCPConfig | shq }} --dir {{ .Dir | shq }} {{ if .Resume }}--resume{{ else }}--session-id{{ end }} {{ .SessionID }}`,
		Dir:     "/abs/demo",
	}

	line, err := Resolve(w, "sess-1", false, "")
	require.NoError(t, err)
	assert.Contains(t, line, shellQuote(filepath.Join("/abs/demo", ".mcp.json")))
	assert.Contains(t, line, shellQuote("/abs/demo"))
	assert.Contains(t, line, "--session-id sess-1")

	resumed, err := Resolve(w, "sess-1", true, "")
	require.NoError(t, err)
	assert.Contains(t, resumed, "--resume sess-1")
	assert.NotContains(t, resumed, "--session-id")
}

// TestResolveLaunchesAnAgentThisBuildDoesNotKnow is the regression this whole
// schema change exists for: an agent key with no wiring, no preset and no
// probe used to be refused outright (ErrUnknownAgent). Its command is now the
// only thing that matters, and it launches.
func TestResolveLaunchesAnAgentThisBuildDoesNotKnow(t *testing.T) {
	t.Parallel()

	w := Workspace{Command: "pi --some-flag", Dir: "/abs/demo"}
	line, err := Resolve(w, "sess", false, "")
	require.NoError(t, err)
	assert.Equal(t, "cd '/abs/demo' && pi --some-flag", line)
}

func TestResolveRejectsABrokenTemplate(t *testing.T) {
	t.Parallel()

	t.Run("does not parse", func(t *testing.T) {
		t.Parallel()
		_, err := Resolve(Workspace{Command: "claude {{ .SessionID", Dir: "/tmp"}, "s", false, "")
		require.ErrorIs(t, err, ErrCommandTemplate)
	})

	t.Run("names a field LaunchData does not have", func(t *testing.T) {
		t.Parallel()
		_, err := Resolve(Workspace{Command: "claude {{ .Nonsense }}", Dir: "/tmp"}, "s", false, "")
		require.ErrorIs(t, err, ErrCommandTemplate)
	})

	t.Run("renders to nothing", func(t *testing.T) {
		t.Parallel()
		_, err := Resolve(Workspace{Command: "{{ if .Resume }}claude{{ end }}", Dir: "/tmp"}, "s", false, "")
		require.ErrorIs(t, err, ErrCommandEmpty)
	})
}

// TestValidateCommandCatchesWhatResolveWouldFailOn is the coupling that keeps
// a bad template out of a manifest: anything Resolve refuses must be refused
// by the editor first, or the failure surfaces a day later at spawn.
func TestValidateCommandCatchesWhatResolveWouldFailOn(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"claude {{ .SessionID", "claude {{ .Nonsense }}", "   ", "{{ if .Resume }}x{{ end }}"} {
		require.Error(t, ValidateCommand(command), "command %q must be refused", command)
	}
	require.NoError(t, ValidateCommand("claude --session-id {{ .SessionID }}"))
}

// TestSupportsResumeComparesBothRenderings pins the semantics the caller
// depends on: it reports whether the two launches actually differ, not
// whether .Resume is mentioned. A template naming .Resume without branching
// would relaunch an identical line, which for a pinned session id is the
// "session id already in use" error, not a resume.
func TestSupportsResumeComparesBothRenderings(t *testing.T) {
	t.Parallel()

	assert.True(t, SupportsResume(`claude {{ if .Resume }}--resume{{ else }}--session-id{{ end }} {{ .SessionID }}`))
	assert.False(t, SupportsResume("codex"), "a command that cannot resume reports so")
	assert.False(t, SupportsResume("claude --session-id {{ .SessionID }}"))
	assert.False(t, SupportsResume("claude --resume-always"), "a literal that merely looks like a branch is not one")
	assert.False(t, SupportsResume("claude {{ .Broken"), "an unparseable template cannot resume")
}

func TestCommandIsDangerousReadsTheCommand(t *testing.T) {
	t.Parallel()

	assert.True(t, CommandIsDangerous("claude --dangerously-skip-permissions"))
	assert.True(t, CommandIsDangerous("codex --dangerously-bypass-approvals-and-sandbox"))
	assert.False(t, CommandIsDangerous("claude --permission-mode acceptEdits"))
	assert.False(t, CommandIsDangerous("pi --unknown-bypass-flag"),
		"an unrecognized bypass reports false: the answer is 'not recognized', never 'safe'")
}

// TestJoinTemplateLinesFoldsTheSource lets a manifest break a long invocation
// across lines. It must fold the SOURCE only — a newline inside an
// interpolated value has to survive, which TestLaunchLineQuotesShellMetacharacters
// proves end to end.
func TestJoinTemplateLinesFoldsTheSource(t *testing.T) {
	t.Parallel()

	w := Workspace{
		Command: "claude\n  --permission-mode acceptEdits\n\n  --session-id {{ .SessionID }}\n",
		Dir:     "/abs/demo",
	}
	line, err := Resolve(w, "sess", false, "")
	require.NoError(t, err)
	assert.Equal(t, "cd '/abs/demo' && claude --permission-mode acceptEdits --session-id sess", line)
}

// TestLaunchLineQuotesShellMetacharacters actually executes the finished
// line through a real shell for each dangerous workspace directory, rather
// than asserting shellQuote's own escaping in isolation: the property that
// matters is that the string never takes effect as shell syntax, and running
// it is the only way to prove that.
func TestLaunchLineQuotesShellMetacharacters(t *testing.T) {
	t.Parallel()

	dangerous := []string{
		"has space",
		"semi;colon",
		"$(subshell)",
		"back`tick`",
		"single'quote",
		"new\nline",
	}

	for _, d := range dangerous {
		t.Run(d, func(t *testing.T) {
			t.Parallel()

			// The line opens with cd into the workspace, so the dangerous name
			// must be a real directory for the rest of it to execute at all.
			dir := filepath.Join(t.TempDir(), d)
			require.NoError(t, os.Mkdir(dir, 0o700))

			w := Workspace{Command: "echo {{ .Dir | shq }}", Dir: dir}
			line, err := Resolve(w, "sess", false, "")
			require.NoError(t, err)

			out, err := exec.Command("sh", "-c", line).CombinedOutput()
			require.NoError(t, err, string(out))
			assert.Contains(t, string(out), d)
		})
	}
}

// TestLaunchLineStartsInTheWorkspaceDirectory proves the cd survives a shell
// whose startup moved elsewhere — the failure mode that motivated it: tmux's
// -c sets the pane's initial directory, but the login shell's profile runs
// before -c's command and may cd away.
func TestLaunchLineStartsInTheWorkspaceDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	line, err := Resolve(Workspace{Command: "pwd", Dir: dir}, "sess", false, "")
	require.NoError(t, err)

	cmd := exec.Command("sh", "-c", line)
	cmd.Dir = os.TempDir() // stand-in for a profile that moved the shell away
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Contains(t, string(out), filepath.Base(dir))
}

// TestShippedPresetsAreLaunchable is the cross-product every preset must
// satisfy, not the presets iterated against themselves: a preset the editor
// offers but Resolve refuses would write an unusable manifest.
func TestShippedPresetsAreLaunchable(t *testing.T) {
	t.Parallel()

	presets := BuiltinPresets()
	require.NotEmpty(t, presets)

	for _, p := range presets {
		t.Run(p.ID, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, ValidateCommand(p.Command))

			line, err := Resolve(Workspace{Command: p.Command, Dir: "/abs/demo"}, "sess", false, "")
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(line, "cd '/abs/demo' && "+p.Agent))
			assert.Equal(t, CommandIsDangerous(p.Command), p.Danger,
				"Danger must be derived from the command, never declared beside it")
		})
	}
}

// TestClaudePresetsCarryTheWiringTheTableUsedToAppend: the flags Resolve once
// appended per agent now live in the preset text, so this is what stops them
// being lost in the move.
func TestClaudePresetsCarryTheWiringTheTableUsedToAppend(t *testing.T) {
	t.Parallel()

	for _, p := range BuiltinPresets() {
		if p.Agent != "claude" {
			continue
		}
		assert.Contains(t, p.Command, "--strict-mcp-config", p.ID)
		assert.True(t, SupportsResume(p.Command), "%s must distinguish a resume", p.ID)
	}
}

func TestPresetCommandResolvesTheIDTheSeedNames(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "claude"+claudeTail, PresetCommand("claude-ask"))
	assert.Empty(t, PresetCommand("no-such-preset"))
}

func TestAgentForReadsTheCommandWord(t *testing.T) {
	t.Parallel()

	for command, want := range map[string]string{
		"claude" + claudeTail:                   "claude",
		"  codex --sandbox workspace-write":     "codex",
		"/opt/homebrew/bin/Claude --model opus": "claude",
		"pi":                                    "pi",
		"":                                      "",
		"env FOO=1 claude":                      "env",
	} {
		assert.Equal(t, want, AgentFor(command), command)
	}
}

// TestResolvePassesThePromptThroughEveryPreset pins the contract a scheduled
// launch rests on: the prompt arrives quoted as one trailing word behind the
// end-of-options marker, so a prompt that opens with a hyphen is text rather
// than a flag the CLI rejects, and an empty prompt leaves the interactive line
// untouched byte for byte.
func TestResolvePassesThePromptThroughEveryPreset(t *testing.T) {
	t.Parallel()

	prompts := []string{
		"Say hello; rm -rf /",
		"- Summarize the week\n- Name every open question",
		"--dangerously-skip-permissions",
	}
	for _, p := range BuiltinPresets() {
		t.Run(p.ID, func(t *testing.T) {
			t.Parallel()
			w := Workspace{Command: p.Command, Dir: "/abs/demo"}
			assert.True(t, SupportsPrompt(p.Command))

			bare, err := Resolve(w, "sess-1", false, "")
			require.NoError(t, err)
			for _, prompt := range prompts {
				prompted, err := Resolve(w, "sess-1", false, prompt)
				require.NoError(t, err)
				assert.Equal(t, bare+" -- "+shellQuote(prompt), prompted)
			}
		})
	}
}

// A template that never mentions .Prompt launches the same line either way:
// the prompt is dropped, never spliced somewhere the shell would run it.
func TestSupportsPromptRequiresQuotedPromptValue(t *testing.T) {
	t.Parallel()

	assert.False(t, SupportsPrompt("claude"))
	assert.False(t, SupportsPrompt("claude {{ .Prompt | len | print | slice 0 0 }}"), "mentioning the field without changing the line is not support")
	assert.False(t, SupportsPrompt("claude {{ if .Prompt }}--quiet{{ end }}"), "branching on the prompt without carrying its value is not support")
	assert.False(t, SupportsPrompt("claude {{ .Prompt }}"), "unquoted prompt text would become shell syntax")
	assert.True(t, SupportsPrompt("pi"+PromptTail))

	w := Workspace{Command: "pi --some-flag", Dir: "/abs/demo"}
	bare, err := Resolve(w, "sess", false, "")
	require.NoError(t, err)
	prompted, err := Resolve(w, "sess", false, "hello")
	require.NoError(t, err)
	assert.Equal(t, bare, prompted)
}
