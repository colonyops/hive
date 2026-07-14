package ghcli

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchDriverListArgs(t *testing.T) {
	tests := []struct {
		name   string
		driver searchDriver
		scope  string
		query  string
		limit  int
		want   []string
	}{
		{
			name:   "unscoped issues",
			driver: SearchIssues("triage", "Triage").(searchDriver),
			query:  "label:triage no:assignee",
			limit:  30,
			want: []string{
				"search", "issues", "label:triage no:assignee",
				"--json", "number,title,state,author,labels,url,createdAt,assignees,repository",
				"--limit", "30",
			},
		},
		{
			name:   "scoped pull requests",
			driver: SearchPRs("reviews", "Review Queue").(searchDriver),
			scope:  "owner/repo",
			query:  "review-requested:@me",
			limit:  12,
			want: []string{
				"search", "prs", "review-requested:@me",
				"--repo", "owner/repo",
				"--json", "number,title,state,author,labels,url,createdAt,assignees,repository,isDraft",
				"--limit", "12",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.driver.ListArgs(tt.scope, tt.query, tt.limit))
		})
	}
}

func TestSearchDriverConfig(t *testing.T) {
	cfg := SearchIssues("security-backlog", "Security Backlog").Config()
	assert.Equal(t, "security-backlog", cfg.ID)
	assert.Equal(t, "Security Backlog", cfg.DisplayName)
	assert.Equal(t, "gh", cfg.Binary)
	assert.True(t, cfg.ScopeOptional)
}

func TestSearchDriverParseList(t *testing.T) {
	fixClock(t, time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC))

	tests := []struct {
		name       string
		driver     searchDriver
		fixture    string
		wantID     string
		wantTitle  string
		wantSub    string
		wantURI    string
		wantFields map[string]any
	}{
		{
			name:      "issues",
			driver:    SearchIssues("triage", "Triage").(searchDriver),
			fixture:   "testdata/search_issues.json",
			wantID:    "123",
			wantTitle: "Global issue",
			wantSub:   "#123 · OPEN",
			wantURI:   "https://github.com/owner/widgets/issues/123",
			wantFields: map[string]any{
				"number":         123,
				"title":          "Global issue",
				"state":          "OPEN",
				"url":            "https://github.com/owner/widgets/issues/123",
				"author":         "alice",
				"labels":         []string{"bug", "triage"},
				"age":            "3d",
				"assignee":       "carol",
				"assignee_count": 1,
				"repo":           "owner/widgets",
			},
		},
		{
			name:      "pull requests",
			driver:    SearchPRs("reviews", "Review Queue").(searchDriver),
			fixture:   "testdata/search_prs.json",
			wantID:    "456",
			wantTitle: "Cross-repo change",
			wantSub:   "#456 · draft",
			wantURI:   "https://github.com/other/service/pull/456",
			wantFields: map[string]any{
				"number":         456,
				"title":          "Cross-repo change",
				"state":          "OPEN",
				"url":            "https://github.com/other/service/pull/456",
				"author":         "bob",
				"labels":         []string{"dependencies"},
				"draft":          true,
				"age":            "5d",
				"assignee":       "",
				"assignee_count": 0,
				"repo":           "other/service",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := os.ReadFile(tt.fixture)
			require.NoError(t, err)

			items, err := tt.driver.ParseList(payload)
			require.NoError(t, err)
			require.Len(t, items, 1)
			assert.Equal(t, tt.wantID, items[0].ID)
			assert.Equal(t, tt.wantTitle, items[0].Title)
			assert.Equal(t, tt.wantSub, items[0].Subtitle)
			assert.Equal(t, tt.wantURI, items[0].URI)
			assert.Equal(t, tt.wantFields, items[0].Fields)
			assert.NotContains(t, items[0].Fields, "review")
			assert.NotContains(t, items[0].Fields, "ci")
			assert.NotContains(t, items[0].Fields, "linked_pr")
			assert.NotContains(t, items[0].Fields, "linked_issue")
		})
	}
}

func TestSearchDriverDetail(t *testing.T) {
	tests := []struct {
		name   string
		driver searchDriver
		want   []string
	}{
		{
			name:   "issue",
			driver: SearchIssues("triage", "Triage").(searchDriver),
			want:   []string{"issue", "view", "123", "--repo", "owner/repo", "--json", "number,title,body,url,state"},
		},
		{
			name:   "pull request",
			driver: SearchPRs("reviews", "Review Queue").(searchDriver),
			want:   []string{"pr", "view", "123", "--repo", "owner/repo", "--json", "number,title,body,url,state"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.driver.DetailArgs("owner/repo", "123"))
			detail, err := tt.driver.ParseDetail([]byte(`{"number":123,"title":"Title","body":"body text","url":"https://github.com/owner/repo/issues/123","state":"OPEN"}`))
			require.NoError(t, err)
			require.NotNil(t, detail.Markdown)
			assert.Equal(t, "body text", detail.Markdown.Content)
		})
	}
}
