//go:build remote

package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientCLI_UpdatesTagsOnAServerQualifiedID(t *testing.T) {
	created := serverA.CreateSession(t, "qualified-tags")
	serverA.Hive(t, "session", "update", created.ID, "--add-tag", "auto").Ok(t)
	client := NewClient(t)
	ref := serverA.Name + ":" + created.ID

	updated := DecodeJSON[sessionWithServer](t, client.Hive("session", "tags", "update", ref, "--add", "claimed", "--remove", "auto").Ok(t))

	assert.Equal(t, sessionWithServer{Server: serverA.Name, ID: created.ID, Name: "qualified-tags"}, updated)
	assert.Equal(t, []string{"claimed"}, serverA.Show(t, created.ID).Tags)
}

func TestClientCLI_ListFiltersServerSessionsByTag(t *testing.T) {
	tagged := serverA.CreateSession(t, "tag-filter-hit")
	untagged := serverA.CreateSession(t, "tag-filter-miss")
	serverA.Hive(t, "session", "update", tagged.ID, "--add-tag", "pick-me").Ok(t)

	listed := DecodeJSONLines[sessionWithServer](t, NewClient(t).Hive("session", "list", "--json", "--tags", "pick-me").Ok(t))

	assert.Contains(t, listed, sessionWithServer{Server: serverA.Name, ID: tagged.ID, Name: "tag-filter-hit"})
	assert.NotContains(t, listed, sessionWithServer{Server: serverA.Name, ID: untagged.ID, Name: "tag-filter-miss"})
}
