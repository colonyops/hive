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

// SearchItem is one issue or pull request from the batched GraphQL search.
type SearchItem struct {
	Number        int
	Title         string
	Body          string
	State         string
	URL           string // html URL
	Repo          string // "owner/name" (repository.nameWithOwner)
	Author        string // author.login; "" for deleted (ghost) users
	Labels        []Label
	IsPullRequest bool // __typename == "PullRequest"
	Draft         bool // isDraft; always false for issues
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Label is an issue/PR label.
type Label struct {
	Name string `json:"name"`
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
