package teacli

import (
	"fmt"
	"strconv"

	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/sources/cliengine"
)

// prsFields is the tea --fields set the pulls driver requests.
const prsFields = "index,title,state,author,url,created,labels,head,base,mergeable"

// prsDriver is the built-in Gitea/Forgejo pull requests source, backed by
// `tea pulls list`. Gitea exposes no review-decision or CI rollup via tea's
// list output, so those card cells stay blank; the head branch feeds
// .Fields.branch for branch templates.
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
		// Fields keys number/title/url/branch are load-bearing: default source
		// session templates reference .Fields.number, .Fields.url, and
		// .Fields.branch. The card layout reads age/review/labels; review
		// condenses Gitea's state + mergeability since tea exposes no review
		// decision.
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

// reviewLabel condenses a Gitea PR's state and mergeability into one
// human-scannable card cell. tea's list output carries no review decision, so
// this reflects merge status instead: merged/closed states pass through, and
// an open-but-unmergeable PR reads "conflict".
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

// teaPull is the JSON shape of a single entry from
// `tea pulls list --output json --fields index,title,state,author,url,created,labels,head,base,mergeable`.
// Every value is a string (tea's JSON is stringly-typed).
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
