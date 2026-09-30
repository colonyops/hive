package ghclient

import (
	"context"
	"fmt"
	"strings"
)

// BranchRef addresses one branch's pull request.
type BranchRef struct {
	Owner  string
	Repo   string
	Branch string
}

// CheckState condenses a pull request's check rollup. GitHub reports one
// rollup state per commit, so unlike the CLI's per-check array there is
// nothing to fold together here.
type CheckState string

const (
	// CheckStateNone is a head commit with no checks configured.
	CheckStateNone    CheckState = ""
	CheckStatePassing CheckState = "passing"
	CheckStatePending CheckState = "pending"
	CheckStateFailing CheckState = "failing"
)

// ReviewState condenses GitHub's draft and review-decision fields.
type ReviewState string

const (
	ReviewStateOpen             ReviewState = "open"
	ReviewStateDraft            ReviewState = "draft"
	ReviewStateApproved         ReviewState = "approved"
	ReviewStateChangesRequested ReviewState = "changes_requested"
	ReviewStateReviewRequired   ReviewState = "review_required"
)

// PullRequest is the branch's most recently updated pull request. Found is
// false when the branch has none; the caller must keep that distinct from a
// failed lookup, which arrives as an error instead.
type PullRequest struct {
	Found          bool
	Number         int
	State          string // OPEN, CLOSED, MERGED
	IsDraft        bool
	URL            string
	Title          string
	ReviewDecision string // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, or empty
	Checks         CheckState
	// Additions and Deletions are the pull request's own line counts, which are
	// not the working tree's: the branch may have moved since it was opened.
	Additions int
	Deletions int
}

// PullRequestsByBranch resolves each ref's pull request in a single GraphQL
// request, aliased so out[i] answers refs[i].
//
// A repository the token cannot see resolves to a null alias rather than
// failing the batch (postGraphQL's tolerateNotFound), and reports Found: false
// like a branch with no pull request. A transport or auth failure is what
// arrives as an error, for the whole batch.
func (c *Client) PullRequestsByBranch(ctx context.Context, refs []BranchRef) ([]PullRequest, error) {
	if len(refs) == 0 {
		return nil, nil
	}

	doc, variables := buildPullRequestQuery(refs)
	var data map[string]*gqlPRRepository
	if err := c.postGraphQL(ctx, doc, variables, &data, true); err != nil {
		return nil, err
	}

	out := make([]PullRequest, len(refs))
	for i := range refs {
		repo := data[fmt.Sprintf("r%d", i)]
		if repo == nil || len(repo.PullRequests.Nodes) == 0 {
			continue
		}
		node := repo.PullRequests.Nodes[0]
		out[i] = PullRequest{
			Found:          true,
			Number:         node.Number,
			State:          node.State,
			IsDraft:        node.IsDraft,
			URL:            node.URL,
			Title:          node.Title,
			ReviewDecision: node.ReviewDecision,
			Checks:         checkState(node.rollupState()),
			Additions:      node.Additions,
			Deletions:      node.Deletions,
		}
	}
	return out, nil
}

// buildPullRequestQuery constructs the aliased document and its variables.
// Alias rN and variables $oN/$nN/$bN correspond to refs[N]; nothing is
// interpolated into the document, so a branch name is never query syntax.
func buildPullRequestQuery(refs []BranchRef) (doc string, variables map[string]any) {
	variables = make(map[string]any, len(refs)*3)
	var b strings.Builder
	b.WriteString("query (")
	for i := range refs {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "$o%d: String!, $n%d: String!, $b%d: String!", i, i, i)
		variables[fmt.Sprintf("o%d", i)] = refs[i].Owner
		variables[fmt.Sprintf("n%d", i)] = refs[i].Repo
		variables[fmt.Sprintf("b%d", i)] = refs[i].Branch
	}
	b.WriteString(") {\n")
	for i := range refs {
		fmt.Fprintf(&b, "  r%d: repository(owner: $o%d, name: $n%d) {\n", i, i, i)
		fmt.Fprintf(&b, "    pullRequests(headRefName: $b%d, first: 1, orderBy: {field: UPDATED_AT, direction: DESC}) {\n", i)
		b.WriteString(`      nodes {
        number title state isDraft url reviewDecision additions deletions
        commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
      }
    }
  }
`)
	}
	b.WriteString("}\n")
	return b.String(), variables
}

// checkState maps GitHub's StatusState onto this app's vocabulary. Anything
// unrecognized — a state GitHub adds later — reads as pending, never passing.
func checkState(state string) CheckState {
	switch state {
	case "":
		return CheckStateNone
	case "SUCCESS":
		return CheckStatePassing
	case "FAILURE", "ERROR":
		return CheckStateFailing
	default:
		return CheckStatePending
	}
}

func reviewState(draft bool, decision string) ReviewState {
	if draft {
		return ReviewStateDraft
	}
	switch decision {
	case "APPROVED":
		return ReviewStateApproved
	case "CHANGES_REQUESTED":
		return ReviewStateChangesRequested
	case "REVIEW_REQUIRED":
		return ReviewStateReviewRequired
	default:
		return ReviewStateOpen
	}
}

type gqlPRRepository struct {
	PullRequests struct {
		Nodes []gqlPRNode `json:"nodes"`
	} `json:"pullRequests"`
}

type gqlPRNode struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	State          string `json:"state"`
	IsDraft        bool   `json:"isDraft"`
	URL            string `json:"url"`
	ReviewDecision string `json:"reviewDecision"`
	Additions      int    `json:"additions"`
	Deletions      int    `json:"deletions"`
	Commits        struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

// rollupState digs the head commit's rollup out, returning "" for a commit
// with no checks — GitHub nulls the whole rollup rather than reporting a state.
func (n gqlPRNode) rollupState() string {
	if len(n.Commits.Nodes) == 0 {
		return ""
	}
	rollup := n.Commits.Nodes[0].Commit.StatusCheckRollup
	if rollup == nil {
		return ""
	}
	return rollup.State
}
