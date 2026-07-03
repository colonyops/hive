package ghcli

import (
	"fmt"
	"strconv"

	"github.com/colonyops/hive/internal/connectors"
)

// IssuesSpec declares the built-in GitHub issues connector: a two-line
// card list with a markdown detail preview, backed by `gh issue list` /
// `gh issue view`.
func IssuesSpec() Spec {
	return Spec{
		ID:          "issues",
		DisplayName: "GitHub Issues",
		Layout:      connectors.LayoutModeList,
		Columns: []connectors.Column{
			{Key: "number", Label: "#", Width: 6},
			{Key: "title", Label: "Title", Flex: 1},
			{Key: "state", Label: "State", Width: 10},
		},
		ListArgs: func(scope, query string, limit int) []string {
			args := []string{
				"issue", "list",
				"--repo", scope,
				"--json", "number,title,state,author,labels,url",
				"--limit", strconv.Itoa(limit),
			}
			if query != "" {
				args = append(args, "--search", query)
			}
			return args
		},
		ParseList: parseIssueList,
		DetailArgs: func(scope, id string) []string {
			return []string{
				"issue", "view", id,
				"--repo", scope,
				"--json", "number,title,body,url,state",
			}
		},
		ParseDetail: parseIssueDetail,
	}
}

// issueListItem is the JSON shape of a single entry returned by
// `gh issue list --json number,title,state,author,labels,url`.
type issueListItem struct {
	Number int       `json:"number"`
	Title  string    `json:"title"`
	State  string    `json:"state"`
	Author ghAuthor  `json:"author"`
	Labels []ghLabel `json:"labels"`
	URL    string    `json:"url"`
}

// issueDetail is the JSON shape returned by
// `gh issue view <id> --json number,title,body,url,state`.
type issueDetail struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
	State  string `json:"state"`
}

func parseIssueList(out []byte) ([]connectors.Item, error) {
	entries, err := decodeList[issueListItem](out)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	items := make([]connectors.Item, 0, len(entries))
	for _, li := range entries {
		// Fields keys number/title/state/url/author are load-bearing:
		// default connector session templates reference .Fields.number
		// and .Fields.url.
		items = append(items, connectors.Item{
			ID:       strconv.Itoa(li.Number),
			Title:    li.Title,
			Subtitle: fmt.Sprintf("#%d · %s", li.Number, li.State),
			URI:      li.URL,
			Fields: map[string]any{
				"number": li.Number,
				"title":  li.Title,
				"state":  li.State,
				"url":    li.URL,
				"author": li.Author.Login,
				"labels": labelNames(li.Labels),
			},
		})
	}
	return items, nil
}

func parseIssueDetail(out []byte) (connectors.Detail, error) {
	var detail issueDetail
	if err := decodeJSON(out, &detail); err != nil {
		return connectors.Detail{}, err
	}
	return connectors.Detail{
		Markdown: &connectors.MarkdownDetail{Content: detail.Body},
	}, nil
}
