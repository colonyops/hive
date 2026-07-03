package ghcli

import (
	"strconv"

	"github.com/colonyops/hive/internal/connectors"
)

// PRsSpec declares the built-in GitHub pull requests connector: a
// full-width table (no detail preview) of open PRs in the scoped repo,
// backed by `gh pr list`.
func PRsSpec() Spec {
	return Spec{
		ID:          "prs",
		DisplayName: "GitHub Pull Requests",
		Layout:      connectors.LayoutModeTable,
		HidePreview: true,
		Columns: []connectors.Column{
			{Key: "number", Label: "#", Width: 6},
			{Key: "title", Label: "Title", Flex: 1},
			{Key: "author", Label: "Author", Width: 14},
			{Key: "review", Label: "Review", Width: 18},
			{Key: "ci", Label: "CI", Width: 10},
		},
		ListArgs: func(scope, query string, limit int) []string {
			// statusCheckRollup rides along in the same gh call (one
			// GraphQL query) — CI status costs no extra requests.
			args := []string{
				"pr", "list",
				"--repo", scope,
				"--json", "number,title,state,author,labels,url,isDraft,reviewDecision,headRefName,statusCheckRollup",
				"--limit", strconv.Itoa(limit),
			}
			if query != "" {
				args = append(args, "--search", query)
			}
			return args
		},
		ParseList: parsePRList,
	}
}

// prListItem is the JSON shape of a single entry returned by
// `gh pr list --json number,title,state,author,labels,url,isDraft,reviewDecision,headRefName`.
type prListItem struct {
	Number            int       `json:"number"`
	Title             string    `json:"title"`
	State             string    `json:"state"`
	Author            ghAuthor  `json:"author"`
	Labels            []ghLabel `json:"labels"`
	URL               string    `json:"url"`
	IsDraft           bool      `json:"isDraft"`
	ReviewDecision    string    `json:"reviewDecision"`
	HeadRefName       string    `json:"headRefName"`
	StatusCheckRollup []prCheck `json:"statusCheckRollup"`
}

// prCheck is one node of gh's statusCheckRollup. The rollup mixes two
// GraphQL shapes: CheckRun (Status/Conclusion) and StatusContext (State);
// unused fields stay empty per node.
type prCheck struct {
	State      string `json:"state"`      // StatusContext: SUCCESS/FAILURE/ERROR/PENDING/EXPECTED
	Status     string `json:"status"`     // CheckRun: COMPLETED/IN_PROGRESS/QUEUED/...
	Conclusion string `json:"conclusion"` // CheckRun: SUCCESS/FAILURE/SKIPPED/CANCELLED/...
}

// ciLabel condenses a PR's check rollup into one table cell: any failed
// check wins, otherwise any unfinished check reports pending, otherwise
// everything passed. An empty rollup (no CI configured) renders blank.
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

func parsePRList(out []byte) ([]connectors.Item, error) {
	entries, err := decodeList[prListItem](out)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	items := make([]connectors.Item, 0, len(entries))
	for _, pr := range entries {
		// Fields keys number/title/url/author/branch/ci are load-bearing:
		// default connector session templates reference .Fields.number,
		// .Fields.url, and .Fields.branch; the picker's CI column reads
		// .Fields.ci.
		items = append(items, connectors.Item{
			ID:       strconv.Itoa(pr.Number),
			Title:    pr.Title,
			Subtitle: "#" + strconv.Itoa(pr.Number) + " · " + reviewLabel(pr),
			URI:      pr.URL,
			Fields: map[string]any{
				"number": pr.Number,
				"title":  pr.Title,
				"state":  pr.State,
				"url":    pr.URL,
				"author": pr.Author.Login,
				"labels": labelNames(pr.Labels),
				"draft":  pr.IsDraft,
				"review": reviewLabel(pr),
				"ci":     ciLabel(pr.StatusCheckRollup),
				"branch": pr.HeadRefName,
			},
		})
	}
	return items, nil
}
