package teacli

import (
	"fmt"
	"strconv"

	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/sources/cliengine"
)

// issuesFields is the tea --fields set the issues driver requests. Order is
// irrelevant (tea keys the JSON by name); body rides along so a selected
// issue's context is available without a second call — tea has no
// single-issue JSON view, and the body is already in tea's list response.
const issuesFields = "index,title,state,author,url,created,assignees,labels,body"

// issuesDriver is the built-in Gitea/Forgejo issues source, backed by
// `tea issues list`. It carries the issue body in Fields["body"] (there is no
// tea single-issue JSON view) rather than exposing a detail capability.
type issuesDriver struct{}

// Issues returns the built-in Gitea issues driver.
func Issues() cliengine.Driver { return issuesDriver{} }

func (issuesDriver) Config() cliengine.Config {
	return cliengine.Config{
		ID:          "issues",
		DisplayName: "Issues",
		Binary:      "tea",
	}
}

func (issuesDriver) ListArgs(scope, query string, limit int) []string {
	args := []string{
		"issues", "list",
		"--repo", scope,
		"--output", "json",
		"--fields", issuesFields,
		"--limit", strconv.Itoa(limit),
	}
	if query != "" {
		args = append(args, "--keyword", query)
	}
	return args
}

func (issuesDriver) ParseList(out []byte) ([]sources.Item, error) {
	entries, err := cliengine.DecodeList[teaIssue](out)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	items := make([]sources.Item, 0, len(entries))
	for _, li := range entries {
		number, _ := strconv.Atoi(li.Index)
		assignee, assigneeCount := firstCSV(li.Assignees)
		// Fields keys number/title/state/url/body are load-bearing: default
		// source session templates reference .Fields.number and .Fields.url,
		// and the body feeds .Detail on selection. The card layout reads
		// age/assignee/labels.
		items = append(items, sources.Item{
			ID:       li.Index,
			Title:    li.Title,
			Subtitle: fmt.Sprintf("#%s · %s", li.Index, li.State),
			URI:      li.URL,
			Fields: map[string]any{
				"number":         number,
				"title":          li.Title,
				"state":          li.State,
				"url":            li.URL,
				"author":         li.Author,
				"labels":         splitCSV(li.Labels),
				"age":            teaAge(li.Created),
				"assignee":       assignee,
				"assignee_count": assigneeCount,
				"body":           li.Body,
			},
		})
	}
	return items, nil
}

// teaIssue is the JSON shape of a single entry from
// `tea issues list --output json --fields index,title,state,author,url,created,assignees,labels,body`.
// Every value is a string (tea's JSON is stringly-typed).
type teaIssue struct {
	Index     string `json:"index"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Author    string `json:"author"`
	URL       string `json:"url"`
	Created   string `json:"created"`
	Assignees string `json:"assignees"`
	Labels    string `json:"labels"`
	Body      string `json:"body"`
}
