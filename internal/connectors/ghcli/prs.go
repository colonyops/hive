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
		},
		ListArgs: func(scope, query string, limit int) []string {
			args := []string{
				"pr", "list",
				"--repo", scope,
				"--json", "number,title,state,author,labels,url,isDraft,reviewDecision,headRefName",
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
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	State          string    `json:"state"`
	Author         ghAuthor  `json:"author"`
	Labels         []ghLabel `json:"labels"`
	URL            string    `json:"url"`
	IsDraft        bool      `json:"isDraft"`
	ReviewDecision string    `json:"reviewDecision"`
	HeadRefName    string    `json:"headRefName"`
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
		// Fields keys number/title/url/author/branch are load-bearing:
		// default connector session templates reference .Fields.number,
		// .Fields.url, and .Fields.branch.
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
				"branch": pr.HeadRefName,
			},
		})
	}
	return items, nil
}
