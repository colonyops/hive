package ghcli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/sources"
	"github.com/colonyops/hive/internal/sources/cliengine"
)

const searchJSONFields = "number,title,state,author,labels,url,createdAt,assignees,repository"

// searchBase selects the gh search subcommand and result shape.
type searchBase int

const (
	searchIssues searchBase = iota
	searchPRs
)

// searchDriver is the gh search-backed source shared by issue and pull request
// saved views.
type searchDriver struct {
	id          string
	displayName string
	base        searchBase
}

var _ cliengine.DetailDriver = searchDriver{}

// SearchIssues constructs a GitHub issue search driver that supports global
// searches and issue detail views.
func SearchIssues(id, displayName string) cliengine.DetailDriver {
	return searchDriver{id: id, displayName: displayName, base: searchIssues}
}

// SearchPRs constructs a GitHub pull request search driver that supports
// global searches and pull request detail views.
func SearchPRs(id, displayName string) cliengine.DetailDriver {
	return searchDriver{id: id, displayName: displayName, base: searchPRs}
}

func (d searchDriver) Config() cliengine.Config {
	return cliengine.Config{
		ID:            d.id,
		DisplayName:   d.displayName,
		Binary:        "gh",
		ScopeOptional: true,
	}
}

func (d searchDriver) ListArgs(scope, query string, limit int) []string {
	fields := searchJSONFields
	if d.base == searchPRs {
		fields += ",isDraft"
	}

	args := []string{"search", d.base.subcommand(), query}
	if scope != "" {
		args = append(args, "--repo", scope)
	}
	return append(args, "--json", fields, "--limit", strconv.Itoa(limit))
}

func (d searchDriver) ParseList(out []byte) ([]sources.Item, error) {
	entries, err := cliengine.DecodeList[searchListItem](out)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	items := make([]sources.Item, 0, len(entries))
	for _, entry := range entries {
		assignee, assigneeCount := assigneeSummary(entry.Assignees)
		fields := map[string]any{
			"number":         entry.Number,
			"title":          entry.Title,
			"state":          entry.State,
			"url":            entry.URL,
			"author":         entry.Author.Login,
			"labels":         labelNames(entry.Labels),
			"age":            cliengine.ShortAge(entry.CreatedAt),
			"assignee":       assignee,
			"assignee_count": assigneeCount,
			"repo":           entry.Repository.NameWithOwner,
		}

		subtitleState := entry.State
		if d.base == searchPRs {
			fields["draft"] = entry.IsDraft
			subtitleState = strings.ToLower(entry.State)
			if entry.IsDraft {
				subtitleState = "draft"
			}
		}

		items = append(items, sources.Item{
			ID:       strconv.Itoa(entry.Number),
			Title:    entry.Title,
			Subtitle: fmt.Sprintf("#%d · %s", entry.Number, subtitleState),
			URI:      entry.URL,
			Fields:   fields,
		})
	}
	return items, nil
}

func (d searchDriver) DetailArgs(scope, id string) []string {
	if d.base == searchIssues {
		return issuesDriver{}.DetailArgs(scope, id)
	}
	return []string{
		"pr", "view", id,
		"--repo", scope,
		"--json", "number,title,body,url,state",
	}
}

func (searchDriver) ParseDetail(out []byte) (sources.Detail, error) {
	// gh issue view and gh pr view use the same detail payload shape.
	return issuesDriver{}.ParseDetail(out)
}

func (b searchBase) subcommand() string {
	if b == searchPRs {
		return "prs"
	}
	return "issues"
}

// searchListItem is one `gh search issues|prs --json` entry.
type searchListItem struct {
	Number     int              `json:"number"`
	Title      string           `json:"title"`
	State      string           `json:"state"`
	Author     ghAuthor         `json:"author"`
	Labels     []ghLabel        `json:"labels"`
	URL        string           `json:"url"`
	CreatedAt  time.Time        `json:"createdAt"`
	Assignees  []ghAuthor       `json:"assignees"`
	IsDraft    bool             `json:"isDraft"`
	Repository searchRepository `json:"repository"`
}

type searchRepository struct {
	NameWithOwner string `json:"nameWithOwner"`
}
