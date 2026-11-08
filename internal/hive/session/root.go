package session

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/agent"
	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/pkg/tmpl"
)

// RootSpawn asks for a tmux session at a directory that is not a session's
// checkout, such as a project session's root.
type RootSpawn struct {
	Path       string
	Name       string
	Slug       string
	Prompt     string
	Agent      string
	Background bool
	Env        []string
	// AddDirs are directories the agent must be allowed to work in besides
	// Path, passed with the agent's AddDirFlag when the catalog knows one.
	AddDirs []string
}

// SpawnRoot creates the tmux session for spec with the windows of the rules
// that match every remote. With open set it attaches to a tmux session that
// already exists instead of failing.
func (s *Service) SpawnRoot(ctx context.Context, spec RootSpawn, open bool) error {
	strategy := config.ResolveSpawn(s.config.Rules, "")
	renderer, err := s.rootRenderer(firstNonEmpty(spec.Agent, strategy.Agent), spec.AddDirs)
	if err != nil {
		return err
	}
	data := SpawnData{
		Path:       spec.Path,
		Name:       spec.Name,
		Prompt:     spec.Prompt,
		Slug:       spec.Slug,
		ContextDir: s.config.ContextDir(),
	}
	if open {
		_, err = s.spawner.OpenWindowsWith(ctx, strategy.Windows, data, spec.Background, "", renderer, spec.Env)
	} else {
		err = s.spawner.SpawnWindowsWith(ctx, strategy.Windows, data, spec.Background, renderer, spec.Env)
	}
	if err != nil {
		return fmt.Errorf("spawn terminal: %w", err)
	}
	return nil
}

func (s *Service) rootRenderer(agentKey string, addDirs []string) (*tmpl.Renderer, error) {
	key := firstNonEmpty(agentKey, s.config.Agents.Default)
	profile, ok := s.config.Agents.Profiles[key]
	if !ok {
		if agentKey != "" {
			return nil, fmt.Errorf("unknown agent %q", agentKey)
		}
		return s.renderer, nil
	}
	command := profile.CommandOrDefault(key)
	flags := profile.ShellFlags()
	if known, ok := agent.Lookup(filepath.Base(command)); ok && known.AddDirFlag != "" {
		for _, dir := range addDirs {
			flags = strings.TrimSpace(flags + " " + known.AddDirFlag + " " + tmpl.ShellQuote(dir))
		}
	}
	return s.renderer.WithAgent(command, key, flags), nil
}

// KillTmuxSession kills the named tmux session.
func (s *Service) KillTmuxSession(ctx context.Context, name string) error {
	return s.lifecycle.KillSession(ctx, multiplexer.Target{Session: name})
}
