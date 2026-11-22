package commands

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/internal/platform/driveapi"
)

func TestWriteThreadsFollowsTheDesignLayout(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	resolvedAt := now.Add(-time.Hour)
	general := driveapi.Thread{
		ID: "019a0000-0000-7000-8000-0000a91c03f2",
		Comments: []driveapi.Comment{
			{Author: driveapi.Author{Name: "Hayden", Email: "h@acme.dev"}, Body: "Ship phase 1 first?", CreatedAt: now.Add(-2 * time.Hour)},
			{Author: driveapi.Author{Name: "Hayden", Email: "h@acme.dev", Agent: "claude"}, Body: "Agreed.", CreatedAt: now.Add(-time.Hour)},
		},
	}
	anchored := driveapi.Thread{
		ID:     "019a0000-0000-7000-8000-00005d0e77b1",
		Anchor: &driveapi.Anchor{StartLine: 2, EndLine: 2, Quote: "keep the old ones"},
		Comments: []driveapi.Comment{
			{Deleted: true},
			{Author: driveapi.Author{Name: "Sam"}, Body: "Too long.", CreatedAt: now.Add(-20 * time.Minute)},
		},
	}
	done := driveapi.Thread{ID: "x-resolved", ResolvedAt: &resolvedAt}
	body := []byte("# Plan\nWe rotate tokens and keep the old ones for 7 days.\n")

	var b strings.Builder
	writeThreads(&b, "plans/a.md", []driveapi.Thread{general, anchored, done}, []driveapi.Thread{general, anchored}, body, now)

	assert.Equal(t, `plans/a.md  2 open, 0 outdated, 1 resolved

#a91c03f2  general  open
  Hayden (h@acme.dev) 2h: Ship phase 1 first?
    Hayden (h@acme.dev) [agent: claude] 1h: Agreed.

#5d0e77b1  L2  open
  > "keep the old ones"
  > in: "We rotate tokens and keep the old ones for 7 days."
  (comment deleted)
    Sam 20m: Too long.
`, b.String())
}

func TestShortID(t *testing.T) {
	assert.Equal(t, "0000a1b2", shortID("019a-0000a1b2"))
	assert.Equal(t, "abc", shortID("abc"))
}
