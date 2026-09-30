package agentws

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
)

// LaunchData is what a workspace's command template renders against. It is
// the whole contract: a template referencing anything else fails to render
// (missingkey=error), so this struct is the documented surface an author
// writes to.
type LaunchData struct {
	// Dir is the absolute workspace directory. The line already cds there, so
	// a template needs this only to name a path inside it.
	Dir string
	// MCPConfig is the generated .mcp.json. It is written for every workspace
	// regardless of which agent the manifest names, so a template may point
	// any CLI that reads claude's format at it.
	MCPConfig string
	// SessionID is a uuid Hive minted. A template that never interpolates it
	// leaves the agent to mint its own, and Hive cannot address the
	// conversation afterward.
	SessionID string
	// Resume is true when this launch is reattaching to a conversation the
	// agent already persisted. A template that does not branch on it reports
	// no resume support at all — see SupportsResume.
	Resume bool
	// Prompt is the optional opening message for a detached chat. A template
	// that never interpolates it cannot run schedules or prompted launches --
	// see SupportsPrompt.
	Prompt string
}

var (
	// ErrCommandTemplate reports a command template that does not parse or
	// does not render.
	ErrCommandTemplate = errors.New("agentws: command template")
	// ErrCommandEmpty reports a template that renders to nothing, which would
	// otherwise produce a line ending in a bare `&&`.
	ErrCommandEmpty = errors.New("agentws: command template rendered empty")
)

// launchFuncs are the helpers a command template may call. shq is the same
// quoting the rest of the line gets, exposed because an interpolated path can
// contain a space and the template author cannot quote it by hand without
// knowing what the value will be.
var launchFuncs = template.FuncMap{
	"shq":  shellQuote,
	"join": strings.Join,
}

func parseCommand(command string) (*template.Template, error) {
	t, err := template.New("command").Funcs(launchFuncs).Option("missingkey=error").Parse(joinTemplateLines(command))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCommandTemplate, err)
	}
	return t, nil
}

// joinTemplateLines folds an authored template onto one line, so a manifest
// can break a long invocation across several for readability. It runs on the
// template source, never on rendered output: a newline inside an interpolated
// value (a workspace directory containing one) must survive into the quoted
// word shq produced, and folding after render would corrupt it into a command
// separator.
//
// Interior spacing within a line is untouched, so a quoted literal in the
// template keeps its own runs of spaces.
func joinTemplateLines(command string) string {
	lines := strings.Split(command, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	return strings.Join(kept, " ")
}

// ValidateCommand reports whether command parses and renders. It executes
// against a probe rather than only parsing, because an undefined field is an
// execution error under missingkey=error, not a parse error — and a template
// that only fails at spawn time is the failure mode this whole change exists
// to remove.
func ValidateCommand(command string) error {
	t, err := parseCommand(command)
	if err != nil {
		return err
	}
	probe := LaunchData{Dir: "/probe", MCPConfig: "/probe/.mcp.json", SessionID: "probe-session"}
	if _, err := render(t, probe); err != nil {
		return err
	}
	return nil
}

func render(t *template.Template, data LaunchData) (string, error) {
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("%w: %w", ErrCommandTemplate, err)
	}
	out := strings.TrimSpace(buf.String())
	if out == "" {
		return "", ErrCommandEmpty
	}
	return out, nil
}

// SupportsResume reports whether command distinguishes a resume from a fresh
// launch. It renders both ways and compares rather than inspecting the parse
// tree: what matters is whether the two launches differ, and a template
// mentioning .Resume without changing its output would relaunch the identical
// line — which for a pinned session id is an error, not a resume.
func SupportsResume(command string) bool {
	t, err := parseCommand(command)
	if err != nil {
		return false
	}
	const probeID = "resume-probe"
	fresh, err := render(t, LaunchData{Dir: "/probe", MCPConfig: "/probe/.mcp.json", SessionID: probeID})
	if err != nil {
		return false
	}
	resumed, err := render(t, LaunchData{Dir: "/probe", MCPConfig: "/probe/.mcp.json", SessionID: probeID, Resume: true})
	if err != nil {
		return false
	}
	return fresh != resumed
}

// SupportsPrompt reports whether command carries a shell-quoted prompt into
// the launch. Detached callers cannot recover a prompt that the template drops,
// and direct unquoted interpolation would let prompt text become shell syntax.
func SupportsPrompt(command string) bool {
	t, err := parseCommand(command)
	if err != nil {
		return false
	}
	data := LaunchData{Dir: "/probe", MCPConfig: "/probe/.mcp.json", SessionID: "probe-session"}
	bare, err := render(t, data)
	if err != nil {
		return false
	}
	const probe = "prompt-probe'; exit 91\nsecond line"
	data.Prompt = probe
	prompted, err := render(t, data)
	if err != nil {
		return false
	}
	return bare != prompted && strings.Contains(prompted, shellQuote(probe))
}

// dangerousFlags are the permission bypasses this build knows by name. The
// posture enum used to make `full` self-labelling; with a free-form command
// the label has to be derived from what the command actually says
// (ADR the-workspace-command-is-a-template).
var dangerousFlags = []string{
	"--dangerously-skip-permissions",
	"--dangerously-bypass-approvals-and-sandbox",
	"--yolo",
	"--full-auto",
}

// CommandIsDangerous reports whether command carries a known permission
// bypass. It is advisory labelling for the UI, never a gate: an unknown CLI's
// own bypass flag is not in the list, so a false answer means "not
// recognized", not "safe".
func CommandIsDangerous(command string) bool {
	for _, flag := range dangerousFlags {
		if strings.Contains(command, flag) {
			return true
		}
	}
	return false
}

// Resolve renders w.Command and wraps it in the login-shell line that runs it.
// w.Dir must be the absolute workspace directory: it reaches the template as
// LaunchData.Dir and backs the cd.
//
// The rendered command is spliced in unquoted. It runs under $SHELL -l -c, so
// the shell parses the author's own words — which is what lets a template
// carry a pipeline, an env prefix, or quoting of its own. Every value Hive
// interpolates is quoted by the template through shq.
//
// An empty prompt produces exactly the line an interactive launch produces.
func Resolve(w Workspace, sessionID string, resume bool, prompt string) (string, error) {
	t, err := parseCommand(w.Command)
	if err != nil {
		return "", err
	}
	command, err := render(t, LaunchData{
		Dir:       w.Dir,
		MCPConfig: filepath.Join(w.Dir, mcpJSONFileName),
		SessionID: sessionID,
		Resume:    resume,
		Prompt:    prompt,
	})
	if err != nil {
		return "", err
	}
	// The line runs under $SHELL -l -c, and a login shell's profile is free to
	// cd somewhere else before -c executes; tmux's -c only sets the pane's
	// initial directory. The explicit cd is what guarantees the agent starts
	// in the workspace regardless of what the user's dotfiles do.
	return "cd " + shellQuote(w.Dir) + " && " + command, nil
}

// shellQuote wraps s for a POSIX login shell: single quotes, with embedded
// single quotes closed and re-opened. Every path Hive interpolates into a
// launch line passes through here, and templates reach it as shq.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
