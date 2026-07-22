package github

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Issue is a hydrated single issue or pull request. State is normalized to
// open/closed; Merged is meaningful only for pull requests.
type Issue struct {
	Number    int
	State     string
	Merged    bool
	UpdatedAt time.Time
}

type restIssue struct {
	Number    int       `json:"number"`
	State     string    `json:"state"`
	Merged    bool      `json:"merged"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetIssue fetches and normalizes one GitHub issue.
func (c *Client) GetIssue(ctx context.Context, owner, repo string, number int) (Issue, error) {
	var raw restIssue
	if err := c.getJSON(ctx, fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, number), nil, &raw); err != nil {
		return Issue{}, err
	}
	return Issue{Number: raw.Number, State: strings.ToLower(raw.State), UpdatedAt: raw.UpdatedAt}, nil
}

// GetPullRequest fetches and normalizes one GitHub pull request.
func (c *Client) GetPullRequest(ctx context.Context, owner, repo string, number int) (Issue, error) {
	var raw restIssue
	if err := c.getJSON(ctx, fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, number), nil, &raw); err != nil {
		return Issue{}, err
	}
	return Issue{Number: raw.Number, State: strings.ToLower(raw.State), Merged: raw.Merged, UpdatedAt: raw.UpdatedAt}, nil
}
