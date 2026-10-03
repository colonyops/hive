package app

import (
	"context"
	"os/exec"

	"github.com/colonyops/hive/internal/platform/execenv"
)

// EditorChoice is one editor the Settings selector offers: a CLI command, the
// display title it is shown as, and whether the command resolves on the
// subprocess PATH right now.
type EditorChoice struct {
	Command string
	Title   string
	Found   bool
}

// knownEditors is the catalogue behind the Settings selector — each one a CLI
// launcher that takes a directory argument and returns immediately. The
// setting stores the command, not an enum, so a YAML-authored command outside
// this list still works; it just labels itself.
var knownEditors = []EditorChoice{
	{Command: "zed", Title: "Zed"},
	{Command: "code", Title: "VS Code"},
	{Command: "cursor", Title: "Cursor"},
	{Command: "subl", Title: "Sublime Text"},
}

// editorTitle maps a configured command onto its display title, falling back
// to the command itself for one outside the known catalogue.
func editorTitle(command string) string {
	for _, e := range knownEditors {
		if e.Command == command {
			return e.Title
		}
	}
	return command
}

// launchEditor runs the configured editor on dir and returns without waiting
// for it. Callers pass a directory they resolved themselves — never one a
// request supplied — because this execs an arbitrary configured program.
func launchEditor(ctx context.Context, env *execenv.Resolver, command, dir string) error {
	if command == "" {
		return Errorf(KindInvalid, "no editor is configured; choose one in Settings › General")
	}
	path, err := env.LookPath(ctx, command)
	if err != nil {
		return Errorf(KindInvalid, "editor %q was not found on PATH; choose another in Settings › General", command)
	}
	// WithoutCancel: the editor must outlive the request that launched it —
	// a request-scoped context would kill it the moment the response is sent.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), path, dir)
	cmd.Env = env.Environ(ctx)
	if err := cmd.Start(); err != nil {
		return Wrap(err, KindInternal, "launching %s", command)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// detectEditors reports the known catalogue with each command resolved
// through lookPath — the resolver's, so the answer matches what a launch
// would actually find (ADR subprocess-environment), not the desktop process's own PATH.
func detectEditors(ctx context.Context, lookPath func(context.Context, string) (string, error)) []EditorChoice {
	if lookPath == nil {
		lookPath = func(_ context.Context, name string) (string, error) { return exec.LookPath(name) }
	}
	choices := make([]EditorChoice, len(knownEditors))
	for i, e := range knownEditors {
		_, err := lookPath(ctx, e.Command)
		e.Found = err == nil
		choices[i] = e
	}
	return choices
}
