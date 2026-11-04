// Package reviews declares the operations an agent uses to review a Code
// session's branch diff, served on the drive MCP server. The hive CLI writes
// the same input with "hive review diff", which needs no running desktop.
package reviews

import (
	"time"

	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/drive"
)

// Toolset is the drive toolset: the reviews are served beside documents, so an
// agent configuration that names the drive toolset gets them without an edit.
var Toolset = drive.Toolset

func All() []ops.Op {
	return []ops.Op{DiffReviewPut, DiffReviewGet, DiffReviewClear}
}

// Kinds and sides as the input spells them. "lines" is the domain's empty kind.
const (
	KindLines = "lines"
	KindFile  = "file"
)

type DiffReviewCommentIn struct {
	Path      string `json:"path"                jsonschema:"The file's path in the repository, as the branch diff lists it."`
	Kind      string `json:"kind,omitempty"      jsonschema:"lines (default) for a run of rows, or file for the whole file, which takes no lines."`
	StartLine int    `json:"startLine,omitempty" jsonschema:"First line of the run. With startSide additions it is the line as the branch has it now; with deletions, as the base had it."`
	EndLine   int    `json:"endLine,omitempty"   jsonschema:"Last line of the run, inclusive. Defaults to startLine."`
	StartSide string `json:"startSide,omitempty" jsonschema:"additions (default) or deletions: which version startLine counts in. Use deletions for a removed line."`
	EndSide   string `json:"endSide,omitempty"   jsonschema:"additions or deletions for endLine. Defaults to startSide."`
	Severity  string `json:"severity,omitempty"  jsonschema:"issue (a bug or a real risk), suggestion (default; a change worth making) or note (context, no change asked)."`
	Text      string `json:"text"                jsonschema:"The comment, in Markdown. Say what to change and why; unless the user dismisses it, it goes to the session's agent with their own comments."`
}

type DiffReviewPutIn struct {
	Caller    string                `json:"caller"              jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Never guess or reuse another value."`
	SessionID string                `json:"sessionId,omitempty" jsonschema:"The Code session whose diff this is, by id (from session_list) or name. Omit it in a hive session to mean your own; a Hive chat must name one."`
	Author    string                `json:"author,omitempty"    jsonschema:"Who the review is from, as the user sees it. A second review from the same author replaces the first. Defaults to the chat's workspace, or to agent in a hive session."`
	Agent     string                `json:"agent,omitempty"     jsonschema:"Which coding agent you are: claude, codex or pi. The Diff view draws its mark beside each comment."`
	Summary   string                `json:"summary,omitempty"   jsonschema:"The overall verdict in a few sentences, in Markdown: what the change does well, and the issues that matter most."`
	Comments  []DiffReviewCommentIn `json:"comments,omitempty"  jsonschema:"The inline comments, in reading order. A review needs a summary or at least one comment."`
}

var DiffReviewPut = ops.Spec[DiffReviewPutIn]{
	Name: "diff_review.put", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSession,
	Summary: "Review a Code session's branch diff",
	Guidance: "Store your review of a Code session's changes: the branch against its base, as the Diff view's Branch comparison " +
		"shows it, which is what a pull request shows. The user reads each comment inline under its lines and dismisses the ones " +
		"they disagree with; Send delivers the rest to the session's agent with their own comments. A review replaces your earlier one for the session, so send the whole review each " +
		"time. Every comment names a file in that diff; a lines comment names rows the diff shows, changed or context, and " +
		"Hive quotes them for you. Read the diff with git diff $(git merge-base HEAD <base>) in the checkout to get the line " +
		"numbers right. A refused review lists every comment that needs fixing, and stores nothing. Keep comments actionable, " +
		"one point each; put praise and the overview in summary. Open the diff tab with tab_open view diff when you are done.",
	Fields: ops.Constraints{"agent": {Enum: []string{"claude", "codex", "pi"}}},
}

type DiffReviewGetIn struct {
	Caller    string `json:"caller"              jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Never guess or reuse another value."`
	SessionID string `json:"sessionId,omitempty" jsonschema:"The Code session whose diff this is, by id (from session_list) or name. Omit it in a hive session to mean your own; a Hive chat must name one."`
}

var DiffReviewGet = ops.Spec[DiffReviewGetIn]{
	Name: "diff_review.get", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset, Caller: ops.CallerRuleSession,
	Summary: "Read the reviews of a Code session's diff",
	Guidance: "List the agent reviews stored for a Code session, each with the comments the user has not dismissed or sent " +
		"yet. A dismissed or sent comment is gone, so compare with what you wrote to see what is still waiting. An empty " +
		"list means no review is waiting.",
}

type DiffReviewClearIn struct {
	Caller    string `json:"caller"              jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Never guess or reuse another value."`
	SessionID string `json:"sessionId,omitempty" jsonschema:"The Code session whose diff this is, by id (from session_list) or name. Omit it in a hive session to mean your own; a Hive chat must name one."`
	Author    string `json:"author,omitempty"    jsonschema:"Remove only this author's review. Omit it to remove every review of the session."`
}

var DiffReviewClear = ops.Spec[DiffReviewClearIn]{
	Name: "diff_review.clear", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSession,
	Summary: "Remove reviews of a Code session's diff",
	Guidance: "Remove the reviews of a Code session: every one, or only author's. Use it to withdraw a review that no longer " +
		"applies; to correct one, send diff_review_put again instead. Answers with how many reviews it removed.",
}

// DomainKind reads the input's kind as the domain spells it.
func DomainKind(kind string) review.DiffCommentKind {
	if kind == KindLines {
		return review.DiffCommentLines
	}
	return review.DiffCommentKind(kind)
}

// DiffReviewOut is a review as every surface prints it.
type DiffReviewOut struct {
	ID        string                 `json:"id"`
	Author    string                 `json:"author"`
	Agent     string                 `json:"agent,omitempty"`
	Head      string                 `json:"head"              jsonschema:"The session's HEAD commit when the review was written."`
	Summary   string                 `json:"summary,omitempty"`
	CreatedAt time.Time              `json:"createdAt"`
	Comments  []DiffReviewCommentOut `json:"comments"`
}

type DiffReviewCommentOut struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	StartSide string `json:"startSide,omitempty"`
	StartLine int    `json:"startLine,omitempty"`
	EndSide   string `json:"endSide,omitempty"`
	EndLine   int    `json:"endLine,omitempty"`
	Quote     string `json:"quote,omitempty"     jsonschema:"The rows the comment covers, as patch text, quoted when the review was written."`
	Severity  string `json:"severity"`
	Text      string `json:"text"`
}

func OutOf(r review.DiffReview) DiffReviewOut {
	comments := make([]DiffReviewCommentOut, 0, len(r.Comments))
	for _, c := range r.Comments {
		kind := string(c.Kind)
		if c.Kind == review.DiffCommentLines {
			kind = KindLines
		}
		comments = append(comments, DiffReviewCommentOut{
			ID: c.ID, Kind: kind, Path: c.Path,
			StartSide: string(c.StartSide), StartLine: c.StartLine, EndSide: string(c.EndSide), EndLine: c.EndLine,
			Quote: c.Quote, Severity: string(c.Severity), Text: c.Text,
		})
	}
	return DiffReviewOut{
		ID: r.ID, Author: r.Author, Agent: r.Agent, Head: r.Head, Summary: r.Summary,
		CreatedAt: r.CreatedAt, Comments: comments,
	}
}
