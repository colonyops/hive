package skillsinstall

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Agent identifies an AI agent to install skills for.
type Agent string

const (
	AgentClaude Agent = "claude"
	AgentCodex  Agent = "codex"
)

// Installer extracts bundled skills/commands and links them into agent config dirs.
type Installer struct {
	ConfigDir string  // e.g. ~/.config/hive
	Agents    []Agent // which agents to symlink for
	Force     bool    // overwrite non-symlinks (for --force flag)
}

// Install extracts embedded assets and creates agent symlinks.
func (i *Installer) Install() error {
	skillsDst := filepath.Join(i.ConfigDir, "skills")
	commandsDst := filepath.Join(i.ConfigDir, "commands")

	if err := i.extractDir("skills", skillsDst); err != nil {
		return fmt.Errorf("extract skills: %w", err)
	}

	if err := i.extractDir("commands", commandsDst); err != nil {
		return fmt.Errorf("extract commands: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	for _, agent := range i.Agents {
		skillsLink, commandsLink := agentLinks(homeDir, agent)

		if err := i.ensureSymlink(skillsDst, skillsLink); err != nil {
			return fmt.Errorf("link skills for %s: %w", agent, err)
		}

		if err := i.ensureSymlink(commandsDst, commandsLink); err != nil {
			return fmt.Errorf("link commands for %s: %w", agent, err)
		}
	}

	return nil
}

// extractDir copies all files from the embedded src subtree into dst on disk.
func (i *Installer) extractDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	return fs.WalkDir(Assets, src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, readErr := Assets.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		return os.WriteFile(target, data, 0o644)
	})
}

// ensureSymlink creates or refreshes a symlink at link pointing to target.
// If link already exists and is a symlink, it is removed and recreated.
// If link exists and is NOT a symlink, it is left alone unless Force is set.
func (i *Installer) ensureSymlink(target, link string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	info, err := os.Lstat(link)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		// Existing symlink — remove and recreate.
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("remove existing symlink: %w", err)
		}
	case err == nil && i.Force:
		// Not a symlink but --force was given.
		if err := os.RemoveAll(link); err != nil {
			return fmt.Errorf("remove existing path: %w", err)
		}
	case err == nil:
		return fmt.Errorf("%s already exists and is not a symlink (use --force to overwrite)", link)
	case !os.IsNotExist(err):
		return fmt.Errorf("stat link path: %w", err)
	}

	return os.Symlink(target, link)
}

// agentLinks returns the skill and command symlink paths for a given agent.
func agentLinks(homeDir string, a Agent) (skillsLink, commandsLink string) {
	switch a {
	case AgentClaude:
		return filepath.Join(homeDir, ".claude", "skills"),
			filepath.Join(homeDir, ".claude", "commands")
	case AgentCodex:
		return filepath.Join(homeDir, ".codex", "skills", "claude"),
			filepath.Join(homeDir, ".codex", "prompts")
	default:
		return "", ""
	}
}

// ParseAgent parses an agent string into an Agent constant. Returns false if unrecognised.
func ParseAgent(s string) (Agent, bool) {
	switch Agent(s) {
	case AgentClaude, AgentCodex:
		return Agent(s), true
	default:
		return "", false
	}
}
