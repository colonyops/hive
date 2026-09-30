package gitea

import (
	"fmt"
	"sort"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/gitea/giteaclient"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/itemtext"
)

// branchPrefix identifies Gitea in a suggested branch name. Forgejo items use
// it too — the connector covers both, and a second prefix would split one
// convention across two names for the same forge API.
const branchPrefix = "gitea"

// Which fetch shape produced an item. Recorded on the payload because the two
// shapes carry different fields, so comparing an item from one against the
// same item from the other would read every missing field as a change.
const (
	OriginSearch        = "search"
	OriginNotifications = "notifications"
)

// Item is a normalized Gitea item (pull request or issue). Its JSON payload is
// stored in a durable inbox row, and its field names are the canonical
// inbox-item contract the feed UI and the filter nodes read — so a Gitea item
// renders and routes exactly like a GitHub one without either connector
// knowing about the other.
type Item struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"` // "PR" | "Issue"
	Repo   string `json:"repo"`
	Num    int    `json:"num"`
	Title  string `json:"title"`
	Author string `json:"author"`
	// State is "open", "closed" or "merged".
	State string `json:"state,omitempty"`
	// Origin is which fetch shape produced the item, OriginSearch or
	// OriginNotifications. The two carry different detail — a notification has
	// no author and no labels — so anything comparing one observation to the
	// next has to know whether they are the same shape.
	Origin string `json:"origin"`
	// UpdatedAt is Gitea's last-updated time, unix milliseconds.
	UpdatedAt int64    `json:"updatedAt"`
	Unread    bool     `json:"unread"`
	Labels    []string `json:"labels"`
	Branch    string   `json:"branch"`
	Body      string   `json:"body"`
	Prompt    string   `json:"prompt"`
	URL       string   `json:"url"`
}

// itemID is an item's stable identity within a source. The account half of the
// credential ref is host-qualified and rides along as the inbox row's source
// scope, so "owner/repo#num" does not have to carry the instance to stay
// unambiguous across two Gitea servers.
func itemID(repo string, num int) string {
	return fmt.Sprintf("%s#%d", repo, num)
}

func issueKind(issue giteaclient.Issue) string {
	if issue.IsPullRequest() {
		return itemtext.KindPR
	}
	return itemtext.KindIssue
}

// searchItems maps a search result onto inbox items.
func searchItems(issues []giteaclient.Issue) []Item {
	out := make([]Item, 0, len(issues))
	for _, issue := range issues {
		kind := issueKind(issue)
		labels := make([]string, len(issue.Labels))
		for i, label := range issue.Labels {
			labels[i] = label.Name
		}
		out = append(out, Item{
			ID:        itemID(issue.Repository.FullName, issue.Number),
			Kind:      kind,
			Repo:      issue.Repository.FullName,
			Num:       issue.Number,
			Title:     issue.Title,
			Author:    issue.User.Login,
			State:     issue.LifecycleState(),
			Origin:    OriginSearch,
			UpdatedAt: issue.UpdatedAt.UnixMilli(),
			Unread:    true, // inbox model: unread until read
			Labels:    labels,
			Branch:    itemtext.Branch(branchPrefix, kind, issue.Number, issue.Title),
			Body:      issue.Body,
			Prompt:    itemtext.Prompt(kind, issue.Title, issue.HTMLURL, issue.Body),
			URL:       issue.HTMLURL,
		})
	}
	return out
}

// notificationItems maps the notification inbox onto inbox items.
//
// Unlike GitHub's, a Gitea notification carries its subject's lifecycle state,
// so a notifications source classifies merges and closes as precisely as a
// search source does. What it does not carry is a reason: there is no
// "review_requested" to read off the thread, so every notification is generic
// activity.
func notificationItems(threads []giteaclient.NotificationThread) []Item {
	out := make([]Item, 0, len(threads))
	for _, thread := range threads {
		kind, ok := notificationKind(thread.Subject.Type)
		if !ok {
			// Commits and repository-level threads: out of scope for a PR/issue
			// feed until the UI has a kind for them.
			continue
		}
		num := thread.Subject.Number()
		if num == 0 {
			continue
		}
		repo := thread.Repository.FullName
		out = append(out, Item{
			ID:        itemID(repo, num),
			Kind:      kind,
			Repo:      repo,
			Num:       num,
			Title:     thread.Subject.Title,
			State:     strings.ToLower(thread.Subject.State),
			Origin:    OriginNotifications,
			UpdatedAt: thread.UpdatedAt.UnixMilli(),
			Unread:    thread.Unread,
			Labels:    []string{},
			Branch:    itemtext.Branch(branchPrefix, kind, num, thread.Subject.Title),
			Body:      fmt.Sprintf("Gitea notification for %s in %s.", strings.ToLower(kind), repo),
			Prompt:    itemtext.Prompt(kind, thread.Subject.Title, thread.Subject.HTMLURL, ""),
			URL:       thread.Subject.HTMLURL,
		})
	}
	return out
}

func notificationKind(subjectType string) (string, bool) {
	switch subjectType {
	case "Pull":
		return itemtext.KindPR, true
	case "Issue":
		return itemtext.KindIssue, true
	default:
		return "", false
	}
}

// mergeItems combines the results of the per-involvement searches into one
// snapshot: deduplicated by item id, newest-updated first — the ordering each
// individual search already came back in — and truncated to the limit the node
// asked for, so a union of three searches is still one page of items.
func mergeItems(results [][]Item, limit int) []Item {
	merged := make([]Item, 0, limit)
	seen := make(map[string]bool)
	for _, items := range results {
		for _, item := range items {
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			merged = append(merged, item)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].UpdatedAt > merged[j].UpdatedAt })
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}
