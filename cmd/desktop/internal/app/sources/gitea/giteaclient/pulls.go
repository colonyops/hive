package giteaclient

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// openPullScanLimit bounds the fallback scan below. A repository with more open
// pull requests than one page loses only the ones onto a non-default base.
const openPullScanLimit = 50

// reviewPageLimit bounds the review list a decision is folded out of. Gitea's
// default page is smaller, and a truncated list can flip the verdict.
const reviewPageLimit = 50

// CheckState condenses a pull request's checks. It is the vocabulary the
// session status bar renders, which ghclient answers in too — the bar shows one
// badge and must not learn which forge filled it in.
type CheckState string

const (
	// CheckStateNone is a head commit with no statuses reported.
	CheckStateNone    CheckState = ""
	CheckStatePassing CheckState = "passing"
	CheckStatePending CheckState = "pending"
	CheckStateFailing CheckState = "failing"
)

// PullRequest is a branch's pull request, in the same shape and vocabulary
// ghclient answers in: State is GitHub's OPEN/CLOSED/MERGED rather than Gitea's
// own open/closed plus a merged flag, and ReviewDecision is derived from the
// review list because Gitea reports no decision of its own.
type PullRequest struct {
	Number         int
	Title          string
	State          string
	IsDraft        bool
	URL            string
	ReviewDecision string
	Checks         CheckState
	Additions      int
	Deletions      int
}

// The review decisions PullRequest.ReviewDecision reports. Gitea spells its own
// change request differently, which is why only one of these doubles as the
// review state read off the API.
const (
	ReviewApproved         = "APPROVED"
	ReviewChangesRequested = "CHANGES_REQUESTED"
	ReviewRequired         = "REVIEW_REQUIRED"

	reviewRequestChanges = "REQUEST_CHANGES"
)

// PullRequestForBranch resolves the pull request whose head is branch. found is
// false when the branch has none — a verdict the caller must keep apart from a
// failed lookup, which arrives as an error.
//
// Gitea has no GraphQL endpoint and its pull list takes no head filter, so
// where GitHub answers this in one query this walks several endpoints: the
// repository for its default branch, the pull request by base and head, then
// its checks and reviews. The by-base-and-head lookup is what finds a pull
// request whose branch has since been deleted — a merge deletes the branch and
// rewrites its head ref, so scanning for the branch name would report a
// freshly merged pull request as none.
func (c *Client) PullRequestForBranch(ctx context.Context, owner, repo, branch string) (PullRequest, bool, error) {
	base, err := c.defaultBranch(ctx, owner, repo)
	if err != nil {
		return PullRequest{}, false, err
	}

	pull, found, err := c.pullByBaseHead(ctx, owner, repo, base, branch)
	if err != nil {
		return PullRequest{}, false, err
	}
	if !found {
		pull, found, err = c.openPullByHead(ctx, owner, repo, branch)
		if err != nil || !found {
			return PullRequest{}, false, err
		}
	}

	out := PullRequest{
		Number:    pull.Number,
		Title:     pull.Title,
		State:     pullState(pull),
		IsDraft:   pull.Draft,
		URL:       pull.HTMLURL,
		Additions: pull.Additions,
		Deletions: pull.Deletions,
	}
	if pull.Head.SHA != "" {
		checks, err := c.combinedStatus(ctx, owner, repo, pull.Head.SHA)
		if err != nil {
			return PullRequest{}, false, err
		}
		out.Checks = checks
	}
	// Only an open pull request has a review decision worth showing: the bar
	// paints a merged or closed one by its state, so its reviews are a request
	// that would change nothing.
	if out.State == "OPEN" {
		decision, err := c.reviewDecision(ctx, owner, repo, pull.Number, pull.requestedReviewers())
		if err != nil {
			return PullRequest{}, false, err
		}
		out.ReviewDecision = decision
	}
	return out, true, nil
}

// apiPullRequest is the subset of Gitea's pull request the status bar reads.
type apiPullRequest struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Merged    bool   `json:"merged"`
	Draft     bool   `json:"draft"`
	HTMLURL   string `json:"html_url"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Head      struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	RequestedReviewers []struct{} `json:"requested_reviewers"`
	RequestedTeams     []struct{} `json:"requested_reviewers_teams"`
}

func (p apiPullRequest) requestedReviewers() int {
	return len(p.RequestedReviewers) + len(p.RequestedTeams)
}

// pullState maps Gitea's state onto the bar's. Gitea reports a merged pull
// request as closed and records the merge separately.
func pullState(p apiPullRequest) string {
	if p.Merged {
		return "MERGED"
	}
	return strings.ToUpper(p.State)
}

func (c *Client) defaultBranch(ctx context.Context, owner, repo string) (string, error) {
	var out struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := c.getJSON(ctx, repoPath(owner, repo), nil, &out); err != nil {
		return "", err
	}
	if out.DefaultBranch == "" {
		return "", c.errs.Errorf("repository %s/%s reports no default branch", owner, repo)
	}
	return out.DefaultBranch, nil
}

func (c *Client) pullByBaseHead(ctx context.Context, owner, repo, base, head string) (apiPullRequest, bool, error) {
	path := fmt.Sprintf("%s/pulls/%s/%s", repoPath(owner, repo), url.PathEscape(base), url.PathEscape(head))
	var pull apiPullRequest
	found, err := c.getOptional(ctx, path, nil, &pull)
	return pull, found, err
}

// openPullByHead is what finds a pull request onto a base other than the
// default branch, which the by-base-and-head lookup cannot address.
func (c *Client) openPullByHead(ctx context.Context, owner, repo, branch string) (apiPullRequest, bool, error) {
	params := url.Values{}
	params.Set("state", "open")
	params.Set("sort", "recentupdate")
	params.Set("limit", strconv.Itoa(openPullScanLimit))

	var pulls []apiPullRequest
	if err := c.getJSON(ctx, repoPath(owner, repo)+"/pulls", params, &pulls); err != nil {
		return apiPullRequest{}, false, err
	}
	for _, pull := range pulls {
		if pull.Head.Ref == branch {
			// The list omits the diff stats, so the match is re-read whole.
			return c.pull(ctx, owner, repo, pull.Number)
		}
	}
	return apiPullRequest{}, false, nil
}

func (c *Client) pull(ctx context.Context, owner, repo string, number int) (apiPullRequest, bool, error) {
	path := fmt.Sprintf("%s/pulls/%d", repoPath(owner, repo), number)
	var pull apiPullRequest
	found, err := c.getOptional(ctx, path, nil, &pull)
	return pull, found, err
}

// combinedStatus reads the head commit's rollup. A commit no CI ever reported
// on answers with an empty status list, not a 404.
func (c *Client) combinedStatus(ctx context.Context, owner, repo, sha string) (CheckState, error) {
	var out struct {
		State      string `json:"state"`
		TotalCount int    `json:"total_count"`
	}
	path := fmt.Sprintf("%s/commits/%s/status", repoPath(owner, repo), url.PathEscape(sha))
	found, err := c.getOptional(ctx, path, nil, &out)
	if err != nil || !found || out.TotalCount == 0 {
		return CheckStateNone, err
	}
	return checkState(out.State), nil
}

// checkState maps Gitea's combined status onto the bar's vocabulary. Anything
// unrecognized reads as pending, never passing.
func checkState(state string) CheckState {
	switch state {
	case "":
		return CheckStateNone
	case "success":
		return CheckStatePassing
	case "failure", "error":
		return CheckStateFailing
	default:
		return CheckStatePending
	}
}

type pullReview struct {
	State       string    `json:"state"`
	Dismissed   bool      `json:"dismissed"`
	SubmittedAt time.Time `json:"submitted_at"`
	User        struct {
		Login string `json:"login"`
	} `json:"user"`
}

func (c *Client) reviewDecision(ctx context.Context, owner, repo string, number, requested int) (string, error) {
	params := url.Values{}
	params.Set("limit", strconv.Itoa(reviewPageLimit))

	var reviews []pullReview
	path := fmt.Sprintf("%s/pulls/%d/reviews", repoPath(owner, repo), number)
	found, err := c.getOptional(ctx, path, params, &reviews)
	if err != nil || !found {
		return "", err
	}
	return foldReviewDecision(reviews, requested), nil
}

// foldReviewDecision derives what GitHub reports directly. Only a reviewer's
// latest review counts, so an approval after a change request approves; a
// dismissed review counts for nothing.
func foldReviewDecision(reviews []pullReview, requested int) string {
	latest := map[string]pullReview{}
	for _, review := range reviews {
		if review.Dismissed || review.User.Login == "" {
			continue
		}
		// COMMENT, PENDING and REQUEST_REVIEW state no position, and letting
		// one through would drop the reviewer's standing verdict.
		if review.State != ReviewApproved && review.State != reviewRequestChanges {
			continue
		}
		if previous, ok := latest[review.User.Login]; ok && previous.SubmittedAt.After(review.SubmittedAt) {
			continue
		}
		latest[review.User.Login] = review
	}

	approved := false
	for _, review := range latest {
		if review.State == reviewRequestChanges {
			return ReviewChangesRequested
		}
		approved = true
	}
	if approved {
		return ReviewApproved
	}
	if requested > 0 {
		return ReviewRequired
	}
	return ""
}

func repoPath(owner, repo string) string {
	return fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo))
}
