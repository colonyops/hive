//go:build remote

package remote

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
)

func TestClientCLI_UpdatesTagsOnAServer(t *testing.T) {
	client := NewClient(t)

	tests := []struct {
		name  string
		flags []string
		want  []string
	}{
		{
			name:  "add",
			flags: []string{"--add-tag", "review"},
			want:  []string{"keep", "auto", "review"},
		},
		{
			name:  "remove ignores case",
			flags: []string{"--remove-tag", "AUTO"},
			want:  []string{"keep"},
		},
		{
			name:  "add and remove",
			flags: []string{"--add-tag", "review", "--remove-tag", "auto"},
			want:  []string{"keep", "review"},
		},
		{
			name:  "clear",
			flags: []string{"--clear-tags"},
			want:  []string{},
		},
		{
			name:  "clear then add without removed",
			flags: []string{"--clear-tags", "--add-tag", "a", "--add-tag", "b", "--remove-tag", "a"},
			want:  []string{"b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created := serverA.CreateSession(t, "cli-tags")
			serverA.Hive(t, "session", "update", created.ID, "--add-tag", "keep", "--add-tag", "auto").Ok(t)

			args := append([]string{"session", "update", "--json", created.ID}, tt.flags...)
			updated := DecodeJSON[Session](t, client.HiveOn(serverA, args...).Ok(t))

			assert.Equal(t, tt.want, updated.Tags)
			assert.Equal(t, tt.want, serverA.Show(t, created.ID).Tags)
		})
	}
}

func TestClientCLI_RejectsAnInvalidTagBeforeWriting(t *testing.T) {
	created := serverA.CreateSession(t, "cli-bad-tag")
	serverA.Hive(t, "session", "update", created.ID, "--add-tag", "keep").Ok(t)

	tooLong := strings.Repeat("x", session.MaxTagLength+1)
	failed := NewClient(t).HiveOn(serverA, "session", "update", created.ID, "--clear-tags", "--add-tag", tooLong).Fails(t)

	assert.Contains(t, failed.Stderr, session.ErrInvalidTag.Error())
	assert.Equal(t, []string{"keep"}, serverA.Show(t, created.ID).Tags)
}

// EditTags runs on the daemon in one transaction, so it keeps a tag that
// another client added after this client last read the session.
func TestSessionAPI_EditKeepsATagAnotherClientAdded(t *testing.T) {
	created := serverA.CreateSession(t, "shared-tags")
	api := serverA.API(t)
	ctx := t.Context()

	_, err := api.SetSessionTags(ctx, created.ID, []string{"first"})
	require.NoError(t, err)
	serverA.Hive(t, "session", "update", created.ID, "--add-tag", "from-cli").Ok(t)

	edited, err := api.EditSessionTags(ctx, created.ID, []string{"second"}, []string{"first"})
	require.NoError(t, err)
	assert.Equal(t, []string{"from-cli", "second"}, edited.Tags)
	assert.Equal(t, []string{"from-cli", "second"}, serverA.Show(t, created.ID).Tags)
}

// The CLI checks tags before it sends them, so only the API reaches the
// daemon's InvalidTagDetail error detail.
func TestSessionAPI_RejectsAnInvalidTag(t *testing.T) {
	created := serverA.CreateSession(t, "bad-tag")
	api := serverA.API(t)
	ctx := t.Context()

	_, err := api.SetSessionTags(ctx, created.ID, []string{"keep"})
	require.NoError(t, err)

	_, err = api.SetSessionTags(ctx, created.ID, []string{"a,b"})
	require.ErrorIs(t, err, session.ErrInvalidTag)
	_, err = api.EditSessionTags(ctx, created.ID, []string{"a,b"}, nil)
	require.ErrorIs(t, err, session.ErrInvalidTag)

	assert.Equal(t, []string{"keep"}, serverA.Show(t, created.ID).Tags)
}
