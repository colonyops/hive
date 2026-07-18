package github

import (
	"strconv"
	"strings"
	"time"
)

// User is the authenticated GitHub user.
type User struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// SearchResult is the response of the issue/PR search API.
type SearchResult struct {
	TotalCount int          `json:"total_count"`
	Items      []SearchItem `json:"items"`
}

// SearchItem is one issue or pull request from the search API.
type SearchItem struct {
	Number        int          `json:"number"`
	Title         string       `json:"title"`
	Body          string       `json:"body"`
	State         string       `json:"state"`
	HTMLURL       string       `json:"html_url"`
	RepositoryURL string       `json:"repository_url"`
	User          User         `json:"user"`
	Labels        []Label      `json:"labels"`
	PullRequest   *PullRequest `json:"pull_request"`
	Draft         bool         `json:"draft"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

// Label is an issue/PR label.
type Label struct {
	Name string `json:"name"`
}

// PullRequest marks a search item as a pull request; the search API includes
// the key only for PRs.
type PullRequest struct {
	HTMLURL string `json:"html_url"`
}

// IsPullRequest reports whether the search item is a pull request.
func (i SearchItem) IsPullRequest() bool {
	return i.PullRequest != nil
}

// Repo returns the "owner/name" slug parsed from the repository API URL.
func (i SearchItem) Repo() string {
	return repoFromAPIURL(i.RepositoryURL)
}

// Notification is one thread from the notifications inbox.
type Notification struct {
	ID         string              `json:"id"`
	Unread     bool                `json:"unread"`
	Reason     string              `json:"reason"`
	UpdatedAt  time.Time           `json:"updated_at"`
	Subject    NotificationSubject `json:"subject"`
	Repository Repository          `json:"repository"`
}

// NotificationSubject describes what a notification thread is about.
type NotificationSubject struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"` // "Issue" | "PullRequest" | "Release" | ...
}

// Repository identifies the repository a notification belongs to.
type Repository struct {
	FullName string `json:"full_name"`
}

// Number parses the issue/PR number from the subject API URL. Returns 0 for
// subjects without one (releases, discussions).
func (s NotificationSubject) Number() int {
	idx := strings.LastIndexByte(s.URL, '/')
	if idx < 0 {
		return 0
	}
	n, err := strconv.Atoi(s.URL[idx+1:])
	if err != nil {
		return 0
	}
	return n
}

// repoFromAPIURL extracts "owner/name" from URLs like
// https://api.github.com/repos/owner/name[/...].
func repoFromAPIURL(apiURL string) string {
	const marker = "/repos/"
	idx := strings.Index(apiURL, marker)
	if idx < 0 {
		return ""
	}
	slug := apiURL[idx+len(marker):]
	parts := strings.SplitN(slug, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}
