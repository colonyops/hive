package hive

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/pkg/tmpl"
)

// LookPathFunc resolves an executable name to a path, with the semantics of
// exec.LookPath.
type LookPathFunc func(ctx context.Context, name string) (string, error)

func defaultLookPath(_ context.Context, name string) (string, error) {
	return exec.LookPath(name)
}

// AgentNotFoundError reports that the agent command a session spawn would run
// does not resolve to an executable.
type AgentNotFoundError struct {
	Key     string // agent profile key
	Command string // executable the spawn would run
	Err     error
}

func (e *AgentNotFoundError) Error() string {
	if strings.Contains(e.Command, "/") {
		return fmt.Sprintf("agent %q: command %q is not executable: %v", e.Key, e.Command, e.Err)
	}
	return fmt.Sprintf("agent %q: command %q not found on PATH", e.Key, e.Command)
}

func (e *AgentNotFoundError) Unwrap() error {
	return e.Err
}

// SetLookPath replaces the resolver used to check that the agent command
// exists before a session is created. Callers whose own PATH differs from the
// PATH spawned terminals run with (e.g. a GUI app launched outside a login
// shell) should pass a resolver that searches the terminal PATH. A nil fn
// restores exec.LookPath. Call it before the service is in use; it is not
// synchronized with concurrent session creation.
func (s *SessionService) SetLookPath(fn LookPathFunc) {
	if fn == nil {
		fn = defaultLookPath
	}
	s.lookPath = fn
}

// CheckAgent reports whether the command of agent profile agentKey resolves
// to an executable, using the LookPath set with SetLookPath. An empty key
// checks the default agent. It returns *AgentNotFoundError when the command
// does not resolve. Commands whose executable depends on shell expansion, and
// relative paths, are not checked.
func (s *SessionService) CheckAgent(ctx context.Context, agentKey string) error {
	renderer, err := s.rendererForAgent(agentKey)
	if err != nil {
		return err
	}
	return s.checkRendererAgent(ctx, renderer)
}

// checkSpawnAgent checks the agent only when the spawn strategy renders
// agentCommand, so custom windows/spawn setups that do not launch an agent
// are not blocked.
func (s *SessionService) checkSpawnAgent(ctx context.Context, strategy config.SpawnStrategy, renderer *tmpl.Renderer) error {
	if !strategyUsesAgentCommand(strategy) {
		return nil
	}
	return s.checkRendererAgent(ctx, renderer)
}

func (s *SessionService) checkRendererAgent(ctx context.Context, renderer *tmpl.Renderer) error {
	command := renderer.AgentCommand()
	name := agentExecutable(command)
	if name == "" {
		return nil
	}

	if err := s.resolveExecutable(ctx, name); err != nil {
		return &AgentNotFoundError{Key: renderer.AgentWindow(), Command: name, Err: err}
	}
	return nil
}

func (s *SessionService) resolveExecutable(ctx context.Context, name string) error {
	if !strings.Contains(name, "/") {
		_, err := s.lookPath(ctx, name)
		return err
	}

	path := name
	if rest, ok := strings.CutPrefix(name, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("expand ~: %w", err)
		}
		path = filepath.Join(home, rest)
	}
	// Relative paths resolve against the session directory, which does not
	// exist until after the clone, so they cannot be checked here.
	if !filepath.IsAbs(path) {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return errors.New("not an executable file")
	}
	return nil
}

func strategyUsesAgentCommand(strategy config.SpawnStrategy) bool {
	for _, cmd := range strategy.Commands {
		if referencesAgentCommand(cmd) {
			return true
		}
	}
	for _, w := range strategy.Windows {
		if referencesAgentCommand(w.Command) {
			return true
		}
		for _, p := range w.Panes {
			if referencesAgentCommand(p.Command) {
				return true
			}
		}
	}
	return false
}

func referencesAgentCommand(tmplStr string) bool {
	return strings.Contains(tmplStr, "agentCommand")
}

// agentExecutable returns the executable word of an agent command, skipping
// leading VAR=value environment assignments. It returns "" when the word
// depends on shell expansion that cannot be checked without a shell.
func agentExecutable(command string) string {
	for _, field := range strings.Fields(command) {
		if isEnvAssignment(field) {
			continue
		}
		if strings.ContainsAny(field, "$`\"'") {
			return ""
		}
		return field
	}
	return ""
}

func isEnvAssignment(word string) bool {
	name, _, ok := strings.Cut(word, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}
