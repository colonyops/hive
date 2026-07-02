// Package github implements an in-process hive connector for GitHub issues,
// backed by the gh CLI.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/colonyops/hive/internal/connectors"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/pkg/executil"
)

// defaultSearchLimit bounds how many issues a single Search call returns.
const defaultSearchLimit = 30

// Connector implements connectors.Connector for GitHub issues via the gh
// CLI. Search results may optionally be cached; cache may be nil, in which
// case every call shells out to gh.
type Connector struct {
	exec  executil.Executor
	cache searchCache
}

// New constructs a GitHub issues Connector. exec is used to shell out to gh;
// cache may be nil to disable result caching.
func New(exec executil.Executor, cache kv.KV) *Connector {
	return &Connector{
		exec:  exec,
		cache: newSearchCache(cache),
	}
}

// Name returns the connector's stable identifier.
func (c *Connector) Name() string {
	return "github"
}

// Available reports whether the gh CLI is resolvable on PATH.
func (c *Connector) Available(_ context.Context) bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// Initialize returns the connector's picker manifest: a list layout with
// number/title/state columns, markdown detail support, and remote search
// (every query re-invokes gh; there is no local result set to filter).
func (c *Connector) Initialize(_ context.Context) (connectors.Manifest, error) {
	return connectors.Manifest{
		ID:          c.Name(),
		DisplayName: "GitHub Issues",
		Capabilities: connectors.Capabilities{
			FetchDetail: true,
		},
		Picker: connectors.PickerManifest{
			Layout: connectors.LayoutModeList,
			Columns: []connectors.Column{
				{Key: "number", Label: "#", Width: 6},
				{Key: "title", Label: "Title", Flex: 1},
				{Key: "state", Label: "State", Width: 10},
			},
			Search: connectors.SearchManifest{
				Mode: connectors.SearchModeRemote,
			},
		},
	}, nil
}

// Search returns issues in the repository identified by params.Scope
// ("owner/name"), optionally filtered by params.Query. params.Scope is
// required.
func (c *Connector) Search(ctx context.Context, params connectors.SearchParams) (connectors.SearchResult, error) {
	owner, name, err := parseScope(params.Scope)
	if err != nil {
		return connectors.SearchResult{}, err
	}
	scope := owner + "/" + name

	cacheKey := scope + "|" + params.Query
	if items, ok := c.cache.get(ctx, cacheKey); ok {
		return connectors.SearchResult{Items: itemsFromList(items)}, nil
	}

	args := []string{
		"issue", "list",
		"--repo", scope,
		"--json", "number,title,state,author,labels,url",
		"--limit", strconv.Itoa(defaultSearchLimit),
	}
	if params.Query != "" {
		args = append(args, "--search", params.Query)
	}

	out, err := c.exec.Run(ctx, "gh", args...)
	if err != nil {
		return connectors.SearchResult{}, fmt.Errorf("github connector: gh issue list: %w", err)
	}

	var items []issueListItem
	if err := json.Unmarshal(out, &items); err != nil {
		return connectors.SearchResult{}, fmt.Errorf("github connector: decode gh issue list output: %w", err)
	}

	c.cache.set(ctx, cacheKey, items)

	return connectors.SearchResult{Items: itemsFromList(items)}, nil
}

// FetchDetail returns the markdown detail view for a single issue.
// params.Scope ("owner/name") and params.ID (the issue number) are
// required.
func (c *Connector) FetchDetail(ctx context.Context, params connectors.FetchDetailParams) (connectors.Detail, error) {
	owner, name, err := parseScope(params.Scope)
	if err != nil {
		return connectors.Detail{}, err
	}
	scope := owner + "/" + name

	if params.ID == "" {
		return connectors.Detail{}, fmt.Errorf("github connector: fetchDetail requires an id")
	}

	out, err := c.exec.Run(ctx, "gh", "issue", "view", params.ID,
		"--repo", scope,
		"--json", "number,title,body,url,state",
	)
	if err != nil {
		return connectors.Detail{}, fmt.Errorf("github connector: gh issue view: %w", err)
	}

	var detail issueDetail
	if err := json.Unmarshal(out, &detail); err != nil {
		return connectors.Detail{}, fmt.Errorf("github connector: decode gh issue view output: %w", err)
	}

	return connectors.Detail{
		Markdown: &connectors.MarkdownDetail{Content: detail.Body},
	}, nil
}

// parseScope validates that scope has the shape "owner/name" and returns its
// two parts.
func parseScope(scope string) (owner, name string, err error) {
	if scope == "" {
		return "", "", fmt.Errorf("github connector: scope is required (expected owner/name)")
	}
	parts := strings.Split(scope, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("github connector: invalid scope %q: expected owner/name", scope)
	}
	return parts[0], parts[1], nil
}

// itemsFromList maps gh issue list JSON items into connector items.
func itemsFromList(list []issueListItem) []connectors.Item {
	if len(list) == 0 {
		return nil
	}
	items := make([]connectors.Item, 0, len(list))
	for _, li := range list {
		items = append(items, itemFromListEntry(li))
	}
	return items
}

// itemFromListEntry maps a single gh issue list JSON entry into a
// connectors.Item. Fields keys number/title/state/url/author are
// load-bearing: default connector session templates reference
// .Fields.number and .Fields.url.
func itemFromListEntry(li issueListItem) connectors.Item {
	return connectors.Item{
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
		},
	}
}
