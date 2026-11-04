package commands

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/internal/hive/servers"
	"github.com/colonyops/hive/internal/ops"
)

// outputTree is the full command tree with the error rules cmd/hive/cli
// applies.
func outputTree(root *cli.Command, a *app.App) *cli.Command {
	root = registerCommands(root, a)
	ApplyErrorRules(root)
	return root
}

type outputRun struct {
	stdout string
	// reported is what the program prints for the returned error.
	reported string
	code     int
}

func runForOutputTest(t *testing.T, a *app.App, args ...string) outputRun {
	t.Helper()
	var stdout, stderr, reported bytes.Buffer
	root := outputTree(&cli.Command{Name: "hive", Writer: &stdout, ErrWriter: &stderr}, a)
	err := root.Run(context.Background(), append([]string{"hive"}, args...))
	code := ReportError(&reported, err)
	return outputRun{stdout: stdout.String(), reported: reported.String(), code: code}
}

func walkCommands(c *cli.Command, path []string, visit func(path []string, c *cli.Command)) {
	for _, sub := range c.Commands {
		p := append(append([]string(nil), path...), sub.Name)
		visit(p, sub)
		walkCommands(sub, p, visit)
	}
}

func hasFlag(c *cli.Command, name string) (cli.Flag, bool) {
	for _, f := range c.Flags {
		for _, n := range f.Names() {
			if n == name {
				return f, true
			}
		}
	}
	return nil, false
}

func assertJSONLines(t *testing.T, out string) {
	t.Helper()
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		assert.True(t, json.Valid(sc.Bytes()), "stdout line is not JSON: %q", sc.Text())
	}
	require.NoError(t, sc.Err())
}

func assertErrorLine(t *testing.T, r outputRun) ops.Kind {
	t.Helper()
	require.NotEmpty(t, r.reported, "a failing JSON command reports its error")
	lines := strings.Split(strings.TrimSpace(r.reported), "\n")
	require.Len(t, lines, 1, "one error line: %q", r.reported)

	var line struct {
		Kind    ops.Kind `json:"kind"`
		Message string   `json:"message"`
	}
	dec := json.NewDecoder(strings.NewReader(lines[0]))
	dec.DisallowUnknownFields()
	require.NoError(t, dec.Decode(&line), "error line: %q", lines[0])
	assert.NotEmpty(t, line.Message)
	assert.Equal(t, ops.ExitCode(line.Kind), r.code)
	return line.Kind
}

// A flag named jsonl, or a format flag that selects JSON, is the old
// spelling of --json.
func outputTestApp(t *testing.T) *app.App {
	t.Helper()
	a := testApp(t)
	a.Servers = servers.New(zerolog.Nop(), a.SessionAPI(), a.Config.Servers, "test")
	return a
}

func TestJSONFlagSpelling(t *testing.T) {
	root := outputTree(&cli.Command{Name: "hive"}, outputTestApp(t))

	walkCommands(root, nil, func(path []string, c *cli.Command) {
		name := strings.Join(path, " ")
		if f, ok := hasFlag(c, "json"); ok {
			_, isBool := f.(*cli.BoolFlag)
			assert.True(t, isBool, "hive %s: --json must be a bool flag", name)
		}
		_, jsonl := hasFlag(c, "jsonl")
		assert.False(t, jsonl, "hive %s: spell the JSON switch --json", name)

		if _, format := hasFlag(c, "format"); format {
			// x pick --format chooses what --print writes; json is one of four values.
			assert.Equal(t, "x pick", name, "hive %s: spell the JSON switch --json", name)
		}
	})
}

type failingJSONCase struct {
	args []string
	// kind is the error kind the call must report. Empty means the call
	// succeeds.
	kind ops.Kind
	// skip is the reason a command with --json cannot run in a unit test.
	skip string
}

var jsonCommandCases = map[string]failingJSONCase{
	"doctor":          {args: []string{"doctor", "--json"}},
	"drive ls":        {args: []string{"drive", "ls", "--json", "--all", "--shared"}, kind: ops.KindInvalid},
	"drive spaces":    {args: []string{"drive", "spaces", "--json"}},
	"drive push":      {args: []string{"drive", "push", "--json"}, kind: ops.KindInternal},
	"drive pull":      {args: []string{"drive", "pull", "--json"}, kind: ops.KindInternal},
	"drive share":     {args: []string{"drive", "share", "--json"}, kind: ops.KindInternal},
	"session list":    {args: []string{"session", "list", "--json"}},
	"session info":    {args: []string{"session", "info", "--json"}, kind: ops.KindNotFound},
	"session show":    {args: []string{"session", "show", "--json", "missing"}, kind: ops.KindNotFound},
	"session create":  {args: []string{"session", "create", "--json"}, kind: ops.KindInvalid},
	"session update":  {args: []string{"session", "update", "--json", "--name", "x", "missing"}, kind: ops.KindNotFound},
	"session delete":  {args: []string{"session", "delete", "--json", "missing"}, kind: ops.KindNotFound},
	"session recycle": {args: []string{"session", "recycle", "--json", "missing"}, kind: ops.KindNotFound},
	"ls":              {args: []string{"ls", "--json"}},
	"hc list":         {args: []string{"hc", "list", "--json"}},
	"hc show":         {args: []string{"hc", "show", "--json", "missing"}, kind: ops.KindNotFound},
	"hc context":      {args: []string{"hc", "context", "--json", "missing"}, kind: ops.KindNotFound},
	"workspace list":  {args: []string{"workspace", "list", "--json"}},
	"x assess watch":  {args: []string{"x", "assess", "watch", "--json"}, kind: ops.KindInvalid},
	"x assess replay": {args: []string{"x", "assess", "replay", "--json"}, kind: ops.KindInvalid},
	"daemon status":   {skip: "talks to a daemon socket"},
	"servers ls":      {args: []string{"servers", "ls", "--json"}},
	"servers probe":   {args: []string{"servers", "probe", "--json"}, kind: ops.KindInvalid},
}

func TestJSONCommandsWriteJSONLinesAndMapErrorKinds(t *testing.T) {
	a := outputTestApp(t)
	t.Chdir(t.TempDir())

	seen := map[string]bool{}
	root := outputTree(&cli.Command{Name: "hive"}, a)
	walkCommands(root, nil, func(path []string, c *cli.Command) {
		if _, ok := hasFlag(c, "json"); !ok {
			return
		}
		name := strings.Join(path, " ")
		seen[name] = true
		tc, ok := jsonCommandCases[name]
		if !ok {
			t.Errorf("hive %s has --json and no case in jsonCommandCases", name)
			return
		}
		if tc.skip != "" {
			t.Logf("skip hive %s: %s", name, tc.skip)
			return
		}
		t.Run(name, func(t *testing.T) {
			r := runForOutputTest(t, a, tc.args...)
			assertJSONLines(t, r.stdout)
			if tc.kind == "" {
				assert.Zero(t, r.code)
				assert.Empty(t, r.reported)
				return
			}
			assert.Equal(t, tc.kind, assertErrorLine(t, r))
		})
	})
	for name := range jsonCommandCases {
		assert.True(t, seen[name], "jsonCommandCases names hive %s, which has no --json flag", name)
	}
}

func TestSuccessfulJSONRunsWriteOneObjectPerLine(t *testing.T) {
	a := outputTestApp(t)
	t.Chdir(t.TempDir())

	for _, args := range [][]string{{"doctor", "--json"}, {"drive", "spaces", "--json"}, {"config"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			r := runForOutputTest(t, a, args...)
			require.Zero(t, r.code)
			require.NotEmpty(t, r.stdout)
			assertJSONLines(t, r.stdout)
			assert.Len(t, strings.Split(strings.TrimSpace(r.stdout), "\n"), strings.Count(strings.TrimSpace(r.stdout), "\n")+1)
			assert.NotContains(t, r.stdout, "\n  ", "indented JSON")
		})
	}
}

// A command that prints JSON without a flag reports its errors as JSON too.
func TestAlwaysJSONCommandsReportErrorKinds(t *testing.T) {
	a := outputTestApp(t)
	t.Chdir(t.TempDir())

	for name, tc := range map[string]struct {
		args []string
		kind ops.Kind
	}{
		"batch":         {[]string{"batch", "-f", t.TempDir() + "/missing.json"}, ops.KindInvalid},
		"detect":        {[]string{"detect"}, ops.KindInvalid},
		"hc prune":      {[]string{"hc", "prune", "--older-than", "soon"}, ops.KindInvalid},
		"hc update":     {[]string{"hc", "update", "missing", "--status", "done"}, ops.KindNotFound},
		"todo update":   {[]string{"todo", "update", "--status", "bogus", "1"}, ops.KindInvalid},
		"x assess file": {[]string{"x", "assess", "file"}, ops.KindInvalid},
		"desktop op":    {[]string{"desktop", "tab", "open"}, ops.KindInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			r := runForOutputTest(t, a, tc.args...)
			assertJSONLines(t, r.stdout)
			assert.Equal(t, tc.kind, assertErrorLine(t, r))
		})
	}
}

func TestPlainCommandReportsTextError(t *testing.T) {
	a := outputTestApp(t)
	t.Chdir(t.TempDir())

	r := runForOutputTest(t, a, "session", "info")

	assert.Equal(t, ops.ExitCode(ops.KindNotFound), r.code)
	assert.Contains(t, r.reported, "not in a hive session")
	assert.False(t, strings.HasPrefix(r.reported, "{"), "text mode prints text: %q", r.reported)
}
