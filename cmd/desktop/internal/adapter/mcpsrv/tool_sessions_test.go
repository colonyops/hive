package mcpsrv_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

func seedHiveSession(t *testing.T, core *app.App, id, state, labels string, createdAt int64) {
	t.Helper()
	conn := core.HiveConn()
	require.NotNil(t, conn, "the harness has no hive.db to seed")
	var labelsArg any
	if labels != "" {
		labelsArg = labels
	}
	_, err := conn.ExecContext(t.Context(),
		`INSERT INTO sessions (id, name, slug, path, remote, state, clone_strategy, labels, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'acme/repo', ?, 'full', ?, ?, ?)`,
		id, id, id, "/tmp/"+id, state, labelsArg, createdAt, createdAt)
	require.NoError(t, err)
}

type sessionsAnswer struct {
	Sessions []struct {
		ID     string   `json:"id"`
		Labels []string `json:"labels"`
	} `json:"sessions"`
}

func sessionIDs(a sessionsAnswer) []string {
	ids := []string{}
	for _, s := range a.Sessions {
		ids = append(ids, s.ID)
	}
	return ids
}

func TestListSessionsFiltersByLabelsAndState(t *testing.T) {
	core, session := testSession(t)
	seedHiveSession(t, core, "s1", "active", `["auto","backend"]`, 1)
	seedHiveSession(t, core, "s2", "active", `["Auto"]`, 2)
	seedHiveSession(t, core, "s3", "recycled", `["auto"]`, 3)
	seedHiveSession(t, core, "s4", "active", "", 4)

	var all sessionsAnswer
	call(t, session, "list_sessions", struct{}{}, &all)
	assert.Equal(t, []string{"s4", "s3", "s2", "s1"}, sessionIDs(all), "newest first")
	assert.Equal(t, []string{}, all.Sessions[0].Labels, "no labels is an empty list, not null")

	var auto sessionsAnswer
	call(t, session, "list_sessions", map[string]any{"labels": []string{"AUTO"}, "state": "active"}, &auto)
	assert.Equal(t, []string{"s2", "s1"}, sessionIDs(auto))

	var both sessionsAnswer
	call(t, session, "list_sessions", map[string]any{"labels": []string{"auto", "backend"}}, &both)
	assert.Equal(t, []string{"s1"}, sessionIDs(both))

	assert.Contains(t, callErr(t, session, "list_sessions", map[string]any{"state": "paused"}), string(app.KindInvalid))
}

func TestEditSessionLabelsSwapsALabel(t *testing.T) {
	core, session := testSession(t)
	seedHiveSession(t, core, "s1", "active", `["auto","backend"]`, 1)

	var got struct {
		Session struct {
			ID     string   `json:"id"`
			Labels []string `json:"labels"`
		} `json:"session"`
	}
	call(t, session, "edit_session_labels", map[string]any{"sessionId": "s1", "add": []string{"claimed"}, "remove": []string{"Auto"}}, &got)
	assert.Equal(t, []string{"backend", "claimed"}, got.Session.Labels)

	var auto sessionsAnswer
	call(t, session, "list_sessions", map[string]any{"labels": []string{"auto"}}, &auto)
	assert.Empty(t, auto.Sessions, "the swap is persisted")

	assert.Contains(t, callErr(t, session, "edit_session_labels", map[string]any{"sessionId": "s1"}), string(app.KindInvalid))
	assert.Contains(t, callErr(t, session, "edit_session_labels", map[string]any{"sessionId": "s1", "add": []string{"a,b"}}), string(app.KindInvalid))
	assert.Contains(t, callErr(t, session, "edit_session_labels", map[string]any{"sessionId": "nope", "add": []string{"x"}}), string(app.KindNotFound))
}
