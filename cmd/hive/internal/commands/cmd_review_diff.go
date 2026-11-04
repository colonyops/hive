package commands

import (
	"context"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/diffreview"
	"github.com/colonyops/hive/internal/ops/reviews"
	"github.com/colonyops/hive/pkg/iojson"
)

const sessionIDFlag = "session-id"

// diffCmd is "hive review diff": agents' reviews of a session's branch diff,
// written straight to hive.db so it works with no desktop running. The input
// and output are the diff_review MCP tools' own (internal/ops/reviews).
func (cmd *ReviewCmd) diffCmd() *cli.Command {
	sessionFlag := &cli.StringFlag{
		Name:  sessionIDFlag,
		Usage: "the session to review, by id or name (default: the session of the current directory)",
	}
	input := iojson.FileReader[reviews.DiffReviewPutIn]{}
	return &cli.Command{
		Name:  "diff",
		Usage: "Write and read agent reviews of a session's branch diff",
		Description: `An agent's review of a hive session's changes: the branch against its base,
as the Diff view in Hive Desktop shows it. The desktop draws each comment
inline. The user dismisses the ones they disagree with and sends the rest to
the session's agent with their own comments.

put reads the review as JSON from stdin or --file. It replaces the author's
earlier review of the session. Every comment names a file in the branch diff,
and a lines comment names rows of it; hive quotes those rows itself:

  {
    "author": "claude",
    "agent": "claude",
    "summary": "One real bug, two cleanups.",
    "comments": [
      {"path": "internal/a.go", "startLine": 12, "endLine": 14,
       "severity": "issue", "text": "The error is dropped here."},
      {"path": "internal/a.go", "startSide": "deletions", "startLine": 30,
       "text": "Why remove this check?"},
      {"path": "internal/a_test.go", "kind": "file", "text": "Add a failing case."}
    ]
  }

startSide additions counts lines as the branch has them, deletions as the base
had them. severity is issue, suggestion (default) or note. author defaults to the
agent field, then to "agent". A refused review lists every comment to fix and stores nothing.

Examples:
  hive review diff put < review.json
  hive review diff show
  hive review diff clear --author claude`,
		Commands: []*cli.Command{
			{
				Name:     "put",
				Usage:    "Store a review of the session's branch diff",
				Flags:    []cli.Flag{sessionFlag, input.Flag()},
				Metadata: alwaysJSON(),
				Action: func(ctx context.Context, c *cli.Command) error {
					in, err := input.Read()
					if err != nil {
						return invalidf("read the review: %v", err)
					}
					sess, err := cmd.diffSession(ctx, c.String(sessionIDFlag))
					if err != nil {
						return err
					}
					r, err := cmd.app.DiffReviews().Put(ctx, putInput(sess.ID, in))
					if err != nil {
						return err
					}
					return iojson.WriteLine(c.Root().Writer, reviews.OutOf(r))
				},
			},
			{
				Name:     "show",
				Usage:    "Print the session's reviews, one JSON line each",
				Flags:    []cli.Flag{sessionFlag},
				Metadata: alwaysJSON(),
				Action: func(ctx context.Context, c *cli.Command) error {
					sess, err := cmd.diffSession(ctx, c.String(sessionIDFlag))
					if err != nil {
						return err
					}
					list, err := cmd.app.DiffReviews().List(ctx, sess.ID)
					if err != nil {
						return err
					}
					for _, r := range list {
						if err := iojson.WriteLine(c.Root().Writer, reviews.OutOf(r)); err != nil {
							return err
						}
					}
					return nil
				},
			},
			{
				Name:  "clear",
				Usage: "Remove the session's reviews, or only one author's",
				Flags: []cli.Flag{
					sessionFlag,
					&cli.StringFlag{Name: "author", Usage: "remove only this author's review"},
				},
				Metadata: alwaysJSON(),
				Action: func(ctx context.Context, c *cli.Command) error {
					sess, err := cmd.diffSession(ctx, c.String(sessionIDFlag))
					if err != nil {
						return err
					}
					n, err := cmd.app.DiffReviews().Clear(ctx, sess.ID, strings.TrimSpace(c.String("author")))
					if err != nil {
						return err
					}
					return iojson.WriteLine(c.Root().Writer, map[string]int{"removed": n})
				},
			},
		},
	}
}

func putInput(sessionID string, in reviews.DiffReviewPutIn) diffreview.PutInput {
	author := strings.TrimSpace(in.Author)
	if author == "" {
		author = strings.TrimSpace(in.Agent)
	}
	if author == "" {
		author = "agent"
	}
	comments := make([]diffreview.CommentInput, 0, len(in.Comments))
	for _, c := range in.Comments {
		comments = append(comments, diffreview.CommentInput{
			Kind: reviews.DomainKind(c.Kind), Path: c.Path,
			StartSide: review.DiffSide(c.StartSide), StartLine: c.StartLine,
			EndSide: review.DiffSide(c.EndSide), EndLine: c.EndLine,
			Text: c.Text, Severity: review.Severity(c.Severity),
		})
	}
	return diffreview.PutInput{SessionID: sessionID, Author: author, Agent: in.Agent, Summary: in.Summary, Comments: comments}
}

// diffSession finds the session by id first and by name or slug second, so a
// name that looks like another session's id still means the id.
func (cmd *ReviewCmd) diffSession(ctx context.Context, ref string) (session.Session, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		id, err := cmd.app.Sessions().DetectSession(ctx)
		if err != nil {
			return session.Session{}, err
		}
		if id == "" {
			return session.Session{}, invalidf("not in a hive session; run this from a session's checkout or pass --%s", sessionIDFlag)
		}
		ref = id
	}
	sessions, err := cmd.app.Sessions().ListSessions(ctx)
	if err != nil {
		return session.Session{}, err
	}
	for _, byName := range []bool{false, true} {
		for _, s := range sessions {
			if (!byName && s.ID == ref) || (byName && (s.Name == ref || s.Slug == ref)) {
				return s, nil
			}
		}
	}
	return session.Session{}, notFoundf("no hive session has the id or name %q", ref)
}
