package teacli

import (
	"fmt"
	"strconv"

	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/sources/cliengine"
)

const prsFields = "index,title,state,author,url,created,labels,head,base,mergeable"

// prsDriver is the built-in Gitea/Forgejo pull requests source, backed by
// `tea pulls list`.
type prsDriver struct{}

// PRs returns the built-in Gitea pull requests driver.
func PRs() cliengine.Driver { return prsDriver{} }

func (prsDriver) Config() cliengine.Config {
	return cliengine.Config{
		ID:          "prs",
		DisplayName: "Pull Requests",
		Binary:      "tea",
	}
}

func (prsDriver) ListArgs(scope, query string, limit int) []string {
	args := []string{
		"pulls", "list",
		"--repo", scope,
		"--output", "json",
		"--fields", prsFields,
		"--limit", strconv.Itoa(limit),
	}
	if query != "" {
		args = append(args, "--keyword", query)
	}
	return args
}

func (prsDriver) ParseList(out []byte) ([]sources.Item, error) {
	entries, err := cliengine.DecodeList[teaPull](out)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	items := make([]sources.Item, 0, len(entries))
	for _, pr := range entries {
		number, _ := strconv.Atoi(pr.Index)
		// Fields keys are load-bearing: default session templates reference
		// .Fields.number, .Fields.url, and .Fields.branch; the card layout
		// reads the rest.
		items = append(items, sources.Item{
			ID:       pr.Index,
			Title:    pr.Title,
			Subtitle: fmt.Sprintf("#%s · %s", pr.Index, pr.State),
			URI:      pr.URL,
			Fields: map[string]any{
				"number": number,
				"title":  pr.Title,
				"state":  pr.State,
				"url":    pr.URL,
				"author": pr.Author,
				"labels": splitCSV(pr.Labels),
				"review": reviewLabel(pr),
				"branch": pr.Head,
				"base":   pr.Base,
				"age":    teaAge(pr.Created),
			},
		})
	}
	return items, nil
}

// reviewLabel fills the review card cell from PR state and mergeability —
// tea's list output carries no review decision.
func reviewLabel(pr teaPull) string {
	switch pr.State {
	case "merged":
		return "merged"
	case "closed":
		return "closed"
	}
	if pr.Mergeable == "false" {
		return "conflict"
	}
	return "open"
}

// teaPull is one `tea pulls list --output json` entry.
type teaPull struct {
	Index     string `json:"index"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Author    string `json:"author"`
	URL       string `json:"url"`
	Created   string `json:"created"`
	Labels    string `json:"labels"`
	Head      string `json:"head"`
	Base      string `json:"base"`
	Mergeable string `json:"mergeable"`
}
