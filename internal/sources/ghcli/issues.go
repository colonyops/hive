package ghcli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/colonyops/hive/internal/sources"
)

// issuesDriver is the built-in GitHub issues source: a two-line card list
// with a markdown detail body, backed by `gh issue list` / `gh issue view`.
type issuesDriver struct{}

// Issues returns the built-in GitHub issues driver.
func Issues() DetailDriver { return issuesDriver{} }

func (issuesDriver) Config() Config {
	return Config{
		ID:          "issues",
		DisplayName: "GitHub Issues",
	}
}

func (issuesDriver) ListArgs(scope, query string, limit int) []string {
	args := []string{
		"issue", "list",
		"--repo", scope,
		"--json", "number,title,state,author,labels,url,createdAt,assignees,closedByPullRequestsReferences",
		"--limit", strconv.Itoa(limit),
	}
	if query != "" {
		args = append(args, "--search", query)
	}
	return args
}

func (issuesDriver) ParseList(out []byte) ([]sources.Item, error) {
	entries, err := decodeList[issueListItem](out)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	items := make([]sources.Item, 0, len(entries))
	for _, li := range entries {
		// Fields keys number/title/state/url/author are load-bearing:
		// default source session templates reference .Fields.number
		// and .Fields.url. The card layout reads age/linked_pr/assignee.
		assignee, assigneeCount := assigneeSummary(li.Assignees)
		linkedPR, linkedPRCount := firstRef(li.LinkedPRs)
		items = append(items, sources.Item{
			ID:       strconv.Itoa(li.Number),
			Title:    li.Title,
			Subtitle: fmt.Sprintf("#%d · %s", li.Number, li.State),
			URI:      li.URL,
			Fields: map[string]any{
				"number":          li.Number,
				"title":           li.Title,
				"state":           li.State,
				"url":             li.URL,
				"author":          li.Author.Login,
				"labels":          labelNames(li.Labels),
				"age":             shortAge(li.CreatedAt),
				"linked_pr":       linkedPR,
				"linked_pr_count": linkedPRCount,
				"assignee":        assignee,
				"assignee_count":  assigneeCount,
			},
		})
	}
	return items, nil
}

func (issuesDriver) DetailArgs(scope, id string) []string {
	return []string{
		"issue", "view", id,
		"--repo", scope,
		"--json", "number,title,body,url,state",
	}
}

func (issuesDriver) ParseDetail(out []byte) (sources.Detail, error) {
	var detail issueDetail
	if err := decodeJSON(out, &detail); err != nil {
		return sources.Detail{}, err
	}
	return sources.Detail{
		Markdown: &sources.MarkdownDetail{Content: detail.Body},
	}, nil
}

// issueListItem is the JSON shape of a single entry returned by
// `gh issue list --json number,title,state,author,labels,url,createdAt,assignees,closedByPullRequestsReferences`.
type issueListItem struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	Author    ghAuthor   `json:"author"`
	Labels    []ghLabel  `json:"labels"`
	URL       string     `json:"url"`
	CreatedAt time.Time  `json:"createdAt"`
	Assignees []ghAuthor `json:"assignees"`
	LinkedPRs []ghRef    `json:"closedByPullRequestsReferences"`
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
