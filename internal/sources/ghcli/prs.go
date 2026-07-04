package ghcli

import (
	"strconv"
	"time"

	"github.com/colonyops/hive/internal/sources"
)

// prsDriver is the built-in GitHub pull requests source: a two-line card
// list (no detail view) of open PRs, backed by `gh pr list`. The status
// strip beneath each title reads CI, then review, then author.
type prsDriver struct{}

// PRs returns the built-in GitHub pull requests driver.
func PRs() Driver { return prsDriver{} }

func (prsDriver) Config() Config {
	return Config{
		ID:          "prs",
		DisplayName: "GitHub Pull Requests",
	}
}

func (prsDriver) ListArgs(scope, query string, limit int) []string {
	// statusCheckRollup rides along in the same gh call (one GraphQL
	// query) — CI status costs no extra requests.
	args := []string{
		"pr", "list",
		"--repo", scope,
		"--json", "number,title,state,author,labels,url,isDraft,reviewDecision,headRefName,statusCheckRollup,createdAt,assignees,closingIssuesReferences",
		"--limit", strconv.Itoa(limit),
	}
	if query != "" {
		args = append(args, "--search", query)
	}
	return args
}

func (prsDriver) ParseList(out []byte) ([]sources.Item, error) {
	entries, err := decodeList[prListItem](out)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	items := make([]sources.Item, 0, len(entries))
	for _, pr := range entries {
		// Fields keys number/title/url/author/branch/ci are load-bearing:
		// default source session templates reference .Fields.number,
		// .Fields.url, and .Fields.branch; the card layout reads
		// ci/review/age/linked_issue/assignee.
		assignee, assigneeCount := assigneeSummary(pr.Assignees)
		linkedIssue, linkedIssueCount := firstRef(pr.LinkedIssues)
		items = append(items, sources.Item{
			ID:       strconv.Itoa(pr.Number),
			Title:    pr.Title,
			Subtitle: "#" + strconv.Itoa(pr.Number) + " · " + reviewLabel(pr),
			URI:      pr.URL,
			Fields: map[string]any{
				"number":             pr.Number,
				"title":              pr.Title,
				"state":              pr.State,
				"url":                pr.URL,
				"author":             pr.Author.Login,
				"labels":             labelNames(pr.Labels),
				"draft":              pr.IsDraft,
				"review":             reviewLabel(pr),
				"ci":                 ciLabel(pr.StatusCheckRollup),
				"branch":             pr.HeadRefName,
				"age":                shortAge(pr.CreatedAt),
				"linked_issue":       linkedIssue,
				"linked_issue_count": linkedIssueCount,
				"assignee":           assignee,
				"assignee_count":     assigneeCount,
			},
		})
	}
	return items, nil
}

// prListItem is the JSON shape of a single entry returned by
// `gh pr list --json number,title,state,author,labels,url,isDraft,reviewDecision,headRefName,statusCheckRollup,createdAt,assignees,closingIssuesReferences`.
type prListItem struct {
	Number            int        `json:"number"`
	Title             string     `json:"title"`
	State             string     `json:"state"`
	Author            ghAuthor   `json:"author"`
	Labels            []ghLabel  `json:"labels"`
	URL               string     `json:"url"`
	IsDraft           bool       `json:"isDraft"`
	ReviewDecision    string     `json:"reviewDecision"`
	HeadRefName       string     `json:"headRefName"`
	StatusCheckRollup []prCheck  `json:"statusCheckRollup"`
	CreatedAt         time.Time  `json:"createdAt"`
	Assignees         []ghAuthor `json:"assignees"`
	LinkedIssues      []ghRef    `json:"closingIssuesReferences"`
}

// prCheck is one node of gh's statusCheckRollup, which mixes two GraphQL
// shapes: CheckRun and StatusContext; unused fields stay empty per node.
type prCheck struct {
	State      string `json:"state"`      // StatusContext: SUCCESS/FAILURE/ERROR/PENDING/EXPECTED
	Status     string `json:"status"`     // CheckRun: COMPLETED/IN_PROGRESS/QUEUED/...
	Conclusion string `json:"conclusion"` // CheckRun: SUCCESS/FAILURE/SKIPPED/CANCELLED/...
}

// ciLabel condenses a PR's check rollup into one table cell:
// failing > pending > passing. Unknown completed conclusions (STALE,
// empty, future values) count as pending, never passing. An empty
// rollup (no CI configured) renders blank.
func ciLabel(checks []prCheck) string {
	if len(checks) == 0 {
		return ""
	}
	pending := false
	for _, c := range checks {
		switch c.State {
		case "FAILURE", "ERROR":
			return "failing"
		case "PENDING", "EXPECTED":
			pending = true
		}
		switch c.Conclusion {
		case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE":
			return "failing"
		}
		switch c.Status {
		case "IN_PROGRESS", "QUEUED", "PENDING", "WAITING", "REQUESTED":
			pending = true
		case "COMPLETED":
			switch c.Conclusion {
			case "SUCCESS", "SKIPPED", "NEUTRAL":
			default:
				pending = true
			}
		}
	}
	if pending {
		return "pending"
	}
	return "passing"
}

// reviewLabel condenses gh's draft flag and reviewDecision enum into one
// human-scannable table cell.
func reviewLabel(pr prListItem) string {
	if pr.IsDraft {
		return "draft"
	}
	switch pr.ReviewDecision {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		return "changes requested"
	case "REVIEW_REQUIRED":
		return "review required"
	case "":
		return "open"
	default:
		return pr.ReviewDecision
	}
}
