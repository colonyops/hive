package relayformat_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/relayformat"
)

var receivedAt = time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC)

type fixture struct {
	Method      string              `json:"method"`
	Path        string              `json:"path"`
	Query       url.Values          `json:"query"`
	Headers     map[string][]string `json:"headers"`
	ContentType string              `json:"content_type"`
	Body        string              `json:"body"`
}

func load(t *testing.T, name string) relayformat.Delivery {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	require.NoError(t, err)
	var f fixture
	require.NoError(t, json.Unmarshal(data, &f))
	return relayformat.Delivery{
		ID:          "delivery-" + name,
		ReceivedAt:  receivedAt,
		Method:      f.Method,
		Path:        f.Path,
		Query:       f.Query,
		Headers:     f.Headers,
		Body:        []byte(f.Body),
		ContentType: f.ContentType,
	}
}

func hashOf(name string) string {
	sum := sha256.Sum256([]byte("delivery-" + name))
	return hex.EncodeToString(sum[:])
}

func TestFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture  string
		dialect  string
		wantAuto string
		wantKey  string
		want     map[string]any
		absent   []string
	}{
		{
			fixture:  "ntfy-header",
			wantAuto: relayformat.DialectNtfy,
			wantKey:  hashOf("ntfy-header"),
			want: map[string]any{
				"kind": "Notification", "repo": "alerts", "title": "Backup failed",
				"body": "Backup of /home failed after 3 retries", "priority": 5,
				"labels": []any{"warning", "skull"}, "url": "https://home.example.com/backups",
			},
			absent: []string{"state"},
		},
		{
			fixture:  "ntfy-query",
			wantAuto: relayformat.DialectNtfy,
			wantKey:  hashOf("ntfy-query"),
			want: map[string]any{
				"kind": "Notification", "repo": "alerts", "title": "Disk low",
				"body": "Disk at 95% on /volume1", "priority": 4,
				"labels": []any{"disk", "warning"}, "url": "https://nas.example.com",
			},
		},
		{
			fixture:  "ntfy-json",
			wantAuto: relayformat.DialectNtfy,
			wantKey:  hashOf("ntfy-json"),
			want: map[string]any{
				"kind": "Notification", "repo": "alerts", "title": "Low disk",
				"body": "Disk full on nas", "priority": 4,
				"labels": []any{"warning", "cd"}, "url": "https://nas.example.com/storage",
			},
		},
		{
			fixture:  "ntfy-plain",
			dialect:  relayformat.DialectNtfy,
			wantAuto: relayformat.DialectHealthchecks,
			wantKey:  hashOf("ntfy-plain"),
			want: map[string]any{
				"kind": "Notification", "repo": "alerts", "title": "Backup successful", "body": "Backup successful",
			},
			absent: []string{"priority", "labels", "url", "state"},
		},
		{
			fixture:  "gotify-json",
			wantAuto: relayformat.DialectGotify,
			wantKey:  hashOf("gotify-json"),
			want: map[string]any{
				"kind": "Notification", "title": "Backup", "body": "Backup finished in 4m",
				"priority": 5, "url": "https://backup.example.com/runs/9",
			},
			absent: []string{"state"},
		},
		{
			fixture:  "gotify-form",
			wantAuto: relayformat.DialectGotify,
			wantKey:  hashOf("gotify-form"),
			want: map[string]any{
				"kind": "Notification", "title": "Deploy", "body": "Deployed v1.2.0 to prod", "priority": 5,
			},
			absent: []string{"url"},
		},
		{
			fixture:  "hc-plain",
			wantAuto: relayformat.DialectHealthchecks,
			wantKey:  "backup",
			want:     map[string]any{"kind": "Check", "title": "backup", "repo": "healthchecks", "state": "resolved", "pinged_at": "2026-10-11T09:30:00Z"},
			absent:   []string{"ping", "body"},
		},
		{
			fixture:  "hc-start",
			wantAuto: relayformat.DialectHealthchecks,
			wantKey:  "backup",
			want:     map[string]any{"kind": "Check", "ping": "start"},
			absent:   []string{"state"},
		},
		{
			fixture:  "hc-fail",
			wantAuto: relayformat.DialectHealthchecks,
			wantKey:  "backup",
			want:     map[string]any{"kind": "Check", "state": "down", "body": "rsync: connection refused"},
		},
		{
			fixture:  "hc-log",
			wantAuto: relayformat.DialectHealthchecks,
			wantKey:  "backup",
			want:     map[string]any{"kind": "Check", "ping": "log", "body": "step 2 of 3 done"},
			absent:   []string{"state"},
		},
		{
			fixture:  "hc-exit0",
			wantAuto: relayformat.DialectHealthchecks,
			wantKey:  "backup",
			want:     map[string]any{"kind": "Check", "state": "resolved"},
		},
		{
			fixture:  "hc-exit1",
			wantAuto: relayformat.DialectHealthchecks,
			wantKey:  "backup",
			want:     map[string]any{"kind": "Check", "state": "down", "body": "exit status 1"},
		},
		{
			fixture:  "uptime-kuma",
			wantAuto: relayformat.DialectGeneric,
			want:     map[string]any{"kind": relayformat.DefaultKind},
		},
		{
			fixture:  "hive-json",
			wantAuto: relayformat.DialectGeneric,
			wantKey:  "deploy-42",
			want: map[string]any{
				"kind": relayformat.DefaultKind, "title": "Deploy failed", "state": "open", "url": "https://ci.example.com/42",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()
			d := load(t, tc.fixture)

			_, matched, err := relayformat.Parse(relayformat.DialectAuto, d)
			require.NoError(t, err)
			assert.Equal(t, tc.wantAuto, matched)

			dialect := tc.dialect
			if dialect == "" {
				dialect = tc.wantAuto
			}
			items, matched, err := relayformat.Parse(dialect, d)
			require.NoError(t, err)
			assert.Equal(t, dialect, matched)
			require.Len(t, items, 1)
			item := items[0]

			if tc.wantKey != "" {
				assert.Equal(t, tc.wantKey, item.Key)
			}
			assert.Equal(t, item.Key, item.Payload["id"])
			for field, want := range tc.want {
				assert.Equal(t, want, item.Payload[field], field)
			}
			for _, field := range tc.absent {
				assert.NotContains(t, item.Payload, field)
			}
			raw, ok := item.Payload["raw"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, d.Method, raw["method"])
			assert.Equal(t, d.Path, raw["path"])
		})
	}
}

// TestDialectPromises pins the fields each dialect guarantees for every shape
// it accepts, which is what a flow downstream of the node may rely on.
func TestDialectPromises(t *testing.T) {
	t.Parallel()

	all := []string{
		"ntfy-header", "ntfy-query", "ntfy-json", "ntfy-plain", "gotify-json", "gotify-form",
		"hc-plain", "hc-start", "hc-fail", "hc-log", "hc-exit0", "hc-exit1", "uptime-kuma", "hive-json",
	}
	tests := []struct {
		dialect  string
		fixtures []string
		fields   []string
	}{
		{relayformat.DialectGeneric, all, []string{"id", "title"}},
		{relayformat.DialectNtfy, []string{"ntfy-header", "ntfy-query", "ntfy-json", "ntfy-plain"}, []string{"id", "title", "repo", "body"}},
		{relayformat.DialectGotify, []string{"gotify-json", "gotify-form"}, []string{"id", "title", "body"}},
		{relayformat.DialectHealthchecks, []string{"hc-plain", "hc-start", "hc-fail", "hc-log", "hc-exit0", "hc-exit1"}, []string{"id", "title", "kind"}},
		{relayformat.DialectHealthchecks, []string{"hc-plain", "hc-fail", "hc-exit0", "hc-exit1"}, []string{"state"}},
	}
	for _, tc := range tests {
		for _, name := range tc.fixtures {
			t.Run(tc.dialect+"/"+name, func(t *testing.T) {
				t.Parallel()
				items, _, err := relayformat.Parse(tc.dialect, load(t, name))
				require.NoError(t, err)
				require.Len(t, items, 1)
				for _, field := range tc.fields {
					v, ok := items[0].Payload[field].(string)
					assert.True(t, ok && v != "", "%s: %s is %v", name, field, items[0].Payload[field])
				}
			})
		}
	}
}

func TestNtfyPrecedence(t *testing.T) {
	t.Parallel()

	d := relayformat.Delivery{
		ID:      "p",
		Path:    "/alerts",
		Headers: map[string][]string{"Title": {"from header"}, "X-Message-Id": {"msg-1"}},
		Query:   url.Values{"title": {"from query"}, "message": {"query message"}, "priority": {"high"}},
		Body:    []byte(`{"title":"from json","message":"json message","priority":1,"tags":"a, b"}`),
	}
	items, _, err := relayformat.Parse(relayformat.DialectNtfy, d)
	require.NoError(t, err)
	p := items[0].Payload
	assert.Equal(t, "msg-1", items[0].Key)
	assert.Equal(t, "from header", p["title"])
	assert.Equal(t, "query message", p["body"])
	assert.Equal(t, 4, p["priority"])
	assert.Equal(t, []any{"a", "b"}, p["labels"])
}

func TestNtfyTitleFallsBackToMessageThenTopic(t *testing.T) {
	t.Parallel()

	long := "first line that runs on and on past the eighty character limit for a feed title text\nsecond"
	items, _, err := relayformat.Parse(relayformat.DialectNtfy, relayformat.Delivery{ID: "a", Path: "/t", Body: []byte(long)})
	require.NoError(t, err)
	assert.Equal(t, long[:80], items[0].Payload["title"])

	items, _, err = relayformat.Parse(relayformat.DialectNtfy, relayformat.Delivery{ID: "b", Path: "/t"})
	require.NoError(t, err)
	assert.Equal(t, "t", items[0].Payload["title"])
}

func TestNtfyRepeatPublishIsANewItem(t *testing.T) {
	t.Parallel()

	first := load(t, "ntfy-header")
	second := first
	second.ID = "delivery-other"
	a, _, err := relayformat.Parse(relayformat.DialectNtfy, first)
	require.NoError(t, err)
	b, _, err := relayformat.Parse(relayformat.DialectNtfy, second)
	require.NoError(t, err)
	assert.NotEqual(t, a[0].Key, b[0].Key)
}

func TestHealthchecksAtTheRootFallsBackToGeneric(t *testing.T) {
	t.Parallel()

	items, matched, err := relayformat.Parse(relayformat.DialectHealthchecks, relayformat.Delivery{ID: "r", Method: "GET"})
	require.NoError(t, err)
	assert.Equal(t, relayformat.DialectGeneric, matched)
	assert.Equal(t, relayformat.DefaultKind, items[0].Payload["kind"])
}

func TestHealthchecksLongPathTakesTheSlugBeforeTheSuffix(t *testing.T) {
	t.Parallel()

	items, _, err := relayformat.Parse(relayformat.DialectHealthchecks, relayformat.Delivery{ID: "l", Path: "/ping/key123/nightly/fail"})
	require.NoError(t, err)
	assert.Equal(t, "nightly", items[0].Key)
	assert.Equal(t, "down", items[0].Payload["state"])
}

func TestAutoGotifyWithoutTheRedactedKey(t *testing.T) {
	t.Parallel()

	d := load(t, "gotify-form")
	d.Query = nil
	_, matched, err := relayformat.Parse(relayformat.DialectAuto, d)
	require.NoError(t, err)
	assert.Equal(t, relayformat.DialectGotify, matched)
}
