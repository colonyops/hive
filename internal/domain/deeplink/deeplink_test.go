package deeplink

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocation_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		loc  Location
		url  string
	}{
		{
			name: "code session",
			loc:  Location{Area: AreaCode, Slug: "hive-fix-auth"},
			url:  "hive://code/hive-fix-auth",
		},
		{
			name: "code window and canvas",
			loc:  Location{Area: AreaCode, Slug: "web", Window: "@3", Canvas: "plan"},
			url:  "hive://code/web?canvas=plan&window=%403",
		},
		{
			name: "chats workspace",
			loc:  Location{Area: AreaChats, Workspace: "research"},
			url:  "hive://chats/research",
		},
		{
			name: "chat with canvas",
			loc:  Location{Area: AreaChats, Workspace: "research", Chat: "42", Canvas: "notes"},
			url:  "hive://chats/research/42?canvas=notes",
		},
		{
			name: "inbox profile feed",
			loc:  Location{Area: AreaInbox, Profile: "work", Feed: "prs"},
			url:  "hive://inbox/work?feed=prs",
		},
		{
			name: "inbox item",
			loc:  Location{Area: AreaInbox, Profile: "work", Item: 7},
			url:  "hive://inbox/work/7",
		},
		{
			name: "repository canvas",
			loc:  Location{Area: AreaCanvas, Canvas: "release plan", Owner: "colonyops/hive"},
			url:  "hive://canvas/release%20plan?owner=colonyops%2Fhive",
		},
		{
			name: "global canvas",
			loc:  Location{Area: AreaCanvas, Canvas: "todo", Owner: GlobalCanvasOwner},
			url:  "hive://canvas/todo?owner=%40global",
		},
		{
			name: "slug with a slash",
			loc:  Location{Area: AreaCode, Slug: "a/b"},
			url:  "hive://code/a%2Fb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.loc.URL()
			require.NoError(t, err)
			assert.Equal(t, tt.url, got)

			parsed, err := Parse(got)
			require.NoError(t, err)
			assert.Equal(t, tt.loc, parsed)
		})
	}
}

func TestParse_Lenient(t *testing.T) {
	loc, err := Parse("HIVE://Code/web/?window=@1&future=1")
	require.NoError(t, err)
	assert.Equal(t, Location{Area: AreaCode, Slug: "web", Window: "@1"}, loc)
}

func TestParse_Rejects(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "other scheme", raw: "https://code/web"},
		{name: "opaque", raw: "hive:code/web"},
		{name: "unknown area", raw: "hive://flows/work"},
		{name: "code without slug", raw: "hive://code"},
		{name: "code with extra segment", raw: "hive://code/web/extra"},
		{name: "empty segment", raw: "hive://chats/research//42"},
		{name: "inbox item not a number", raw: "hive://inbox/work/abc"},
		{name: "inbox item zero", raw: "hive://inbox/work/0"},
		{name: "canvas without owner", raw: "hive://canvas/plan"},
		{name: "chats canvas without chat", raw: "hive://chats/research?canvas=plan"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.raw)
			assert.Error(t, err)
		})
	}
}

func TestLocation_ValidateRejectsFieldsFromOtherAreas(t *testing.T) {
	_, err := Location{Area: AreaCode, Slug: "web", Profile: "work"}.URL()
	require.ErrorContains(t, err, "inbox item")

	_, err = Location{Area: AreaInbox, Profile: "work", Canvas: "plan"}.URL()
	require.ErrorContains(t, err, "canvas")

	_, err = Location{}.URL()
	require.ErrorContains(t, err, "unknown link area")
}
