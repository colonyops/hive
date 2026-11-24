package commands

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/opscli"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/sessions"
)

// printSessionOp writes the answer of a session operation. It keeps what the
// hand-written session commands wrote: JSON on stdout unless a person reads
// a terminal without --json, offline servers as warnings on stderr, and the
// status line of a change on stderr unless --json is set.
func (cmd *EngineCmd) printSessionOp(c *cli.Command, info ops.Info, out any, human bool) error {
	w, errw := c.Root().Writer, c.Root().ErrWriter
	if list, ok := out.(sessions.SessionListOut); ok {
		for _, failure := range list.Offline {
			_, _ = fmt.Fprintf(errw, "warning: server %s: %s\n", failure.Server, failure.Error)
		}
		if !human {
			return opscli.Print(w, out)
		}
		cmd.printSessionTable(c, list)
		return nil
	}

	if !c.Bool(jsonFlag) {
		if line := statusLine(info, out); line != "" {
			_, _ = fmt.Fprintln(errw, line)
		}
	}
	if !human {
		return opscli.Print(w, out)
	}
	switch info.Name {
	case sessions.SessionGet.Name, sessions.SessionTagsUpdate.Name:
		printSessionHuman(w, out.(sessions.SessionOut))
	case sessions.SessionStart.Name:
		started := out.(sessions.SessionStartOut)
		ref := session.Ref{Server: started.Server, ID: started.ID}
		switch {
		case started.Completed:
			_, _ = fmt.Fprintf(errw, "Session %s: every command finished, so no terminal runs\n", ref)
		case started.Created:
			_, _ = fmt.Fprintf(errw, "Session %s started\n", ref)
		default:
			_, _ = fmt.Fprintf(errw, "Session %s was already running\n", ref)
		}
	}
	return nil
}

func statusLine(info ops.Info, out any) string {
	switch out := out.(type) {
	case sessions.SessionOut:
		switch info.Name {
		case sessions.SessionCreate.Name:
			return "Session created\n  " + out.Path
		case sessions.SessionUpdate.Name:
			return fmt.Sprintf("Session %s updated", session.Ref{Server: out.Server, ID: out.ID})
		}
	case sessions.SessionDeletedOut:
		return fmt.Sprintf("Session %s deleted", session.Ref{Server: out.Server, ID: out.ID})
	case sessions.SessionRecycleOut:
		ref := session.Ref{}
		if out.Deleted != nil {
			ref = session.Ref{Server: out.Deleted.Server, ID: out.Deleted.ID}
		} else if out.Session != nil {
			ref = session.Ref{Server: out.Session.Server, ID: out.Session.ID}
		}
		return fmt.Sprintf("Session %s recycled", ref)
	}
	return ""
}

func (cmd *EngineCmd) printSessionTable(c *cli.Command, list sessions.SessionListOut) {
	w, errw := c.Root().Writer, c.Root().ErrWriter
	showServer := !c.IsSet("server") && cmd.app.Servers != nil && len(cmd.app.Servers.Servers()) > 0
	if len(list.Sessions) == 0 && len(list.Corrupted) == 0 {
		_, _ = fmt.Fprintln(errw, "No sessions found")
		return
	}
	if len(list.Sessions) > 0 {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		header := []string{"REPO", "NAME", "STATE", "PATH"}
		if showServer {
			header = append([]string{"SERVER"}, header...)
		}
		_, _ = fmt.Fprintln(tw, strings.Join(header, "\t"))
		for _, s := range list.Sessions {
			row := []string{s.Repo, s.Name, s.State, s.Path}
			if showServer {
				row = append([]string{s.Server}, row...)
			}
			_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
		}
		_ = tw.Flush()
	}
	if len(list.Corrupted) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(errw, "Found %d corrupted session(s) with invalid git repositories:\n", len(list.Corrupted))
	elsewhere := false
	for _, s := range list.Corrupted {
		if s.Server != session.LocalServer {
			elsewhere = true
		}
		if showServer {
			_, _ = fmt.Fprintf(errw, "  %s: %s (%s)\n", s.Server, s.Repo, s.Path)
		} else {
			_, _ = fmt.Fprintf(errw, "  %s (%s)\n", s.Repo, s.Path)
		}
	}
	_, _ = fmt.Fprintln(w)
	if showServer || elsewhere {
		_, _ = fmt.Fprintln(errw, "Run 'hive prune' on the machine that has them to clean up")
	} else {
		_, _ = fmt.Fprintln(errw, "Run 'hive prune' to clean up")
	}
}

func printSessionHuman(out io.Writer, sess sessions.SessionOut) {
	if sess.Server != session.LocalServer {
		_, _ = fmt.Fprintf(out, "Server:      %s\n", sess.Server)
	}
	_, _ = fmt.Fprintf(out, "Session ID:  %s\n", sess.ID)
	_, _ = fmt.Fprintf(out, "Name:        %s\n", sess.Name)
	_, _ = fmt.Fprintf(out, "Repository:  %s\n", sess.Repo)
	_, _ = fmt.Fprintf(out, "Inbox:       %s\n", sess.Inbox)
	_, _ = fmt.Fprintf(out, "Path:        %s\n", sess.Path)
	_, _ = fmt.Fprintf(out, "State:       %s\n", sess.State)
	if sess.Group != "" {
		_, _ = fmt.Fprintf(out, "Group:       %s\n", sess.Group)
	}
	if len(sess.Tags) > 0 {
		_, _ = fmt.Fprintf(out, "Tags:        %s\n", strings.Join(sess.Tags, ", "))
	}
}
