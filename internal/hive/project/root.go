package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/colonyops/hive/internal/domain/project"
)

// writeRoot lays out a project session's root: one symlink per child
// checkout, an AGENTS.md that names them, and a CLAUDE.md that imports it.
//
// The children stay where CreateSession put them, under the repos dir, so a
// full clone returns to the recycle pool when the project ends. Agents reach
// them through the links.
func writeRoot(ps project.Session, children []Child, instructions string) error {
	if err := os.MkdirAll(ps.Root, 0o755); err != nil {
		return err
	}
	for _, c := range children {
		if err := os.Symlink(c.Session.Path, filepath.Join(ps.Root, c.Dir)); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(ps.Root, "AGENTS.md"), []byte(rootAgentsMD(ps, children, instructions)), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ps.Root, "CLAUDE.md"), []byte("@AGENTS.md\n"), 0o644)
}

func rootAgentsMD(ps project.Session, children []Child, instructions string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", ps.Name)
	b.WriteString("This directory is a hive project session. It is not a git repository. ")
	b.WriteString("Each directory below is a separate git repository with its own branch, ")
	b.WriteString("so run git and hive commands from inside the repository you are changing.\n\n")
	b.WriteString("## Repositories\n\n")
	b.WriteString("| Directory | Remote | Branch |\n| --- | --- | --- |\n")
	for _, c := range children {
		fmt.Fprintf(&b, "| `%s/` | %s | `%s` |\n", c.Dir, c.Session.Remote, c.Branch)
	}
	b.WriteString("\nEach repository may have its own AGENTS.md or CLAUDE.md. Read it before you change files there.\n")
	if s := strings.TrimSpace(instructions); s != "" {
		fmt.Fprintf(&b, "\n## Project instructions\n\n%s\n", s)
	}
	return b.String()
}
