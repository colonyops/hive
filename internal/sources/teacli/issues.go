package teacli

import (
	"fmt"
	"strconv"

	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/sources/cliengine"
)

// body rides along in issuesFields because tea has no single-issue JSON view;
// carrying it in the list response avoids needing a detail capability.
const issuesFields = "index,title,state,author,url,created,assignees,labels,body"

// issuesDriver is the built-in Gitea/Forgejo issues source, backed by
// `tea issues list`.
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
		// Fields keys are load-bearing: default session templates reference
		// .Fields.number and .Fields.url, body feeds .Detail on selection,
		// and the card layout reads the rest.
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

// teaIssue is one `tea issues list --output json` entry.
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
