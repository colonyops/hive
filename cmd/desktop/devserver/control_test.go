package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/devproxy"
)

func testControl(t *testing.T, cfg Config) (http.Handler, *Store, *Pusher) {
	t.Helper()
	require.NoError(t, cfg.normalize())
	cache := testCache(t, cfg.Cache.TTL)
	store := fixedStore(t, cfg.Overlays...)
	proxy := NewProxy(cfg.Upstream, cache, store, zerolog.Nop())
	pusher := NewPusher(cfg.Webhooks, zerolog.Nop())
	return NewControl(cfg, store, cache, proxy, pusher, zerolog.Nop()).Handler(), store, pusher
}

func ctl(t *testing.T, handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestControlActionAppliesOverlay(t *testing.T) {
	handler, store, _ := testControl(t, Config{})

	rec := ctl(t, handler, http.MethodPost, "/_ctl/action",
		`{"repo":"hay-kot/hive-desktop","num":58,"action":"approval-requested"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	overlay, ok := store.Get("hay-kot/hive-desktop#58")
	require.True(t, ok)
	require.NotNil(t, overlay.Reason)
	assert.Equal(t, "approval_requested", *overlay.Reason)
	assert.NotNil(t, overlay.UpdatedAt, "an action must stamp updatedAt or the classifier ignores it")
}

func TestControlMergeActionSetsStateAndAbsence(t *testing.T) {
	handler, store, _ := testControl(t, Config{})

	rec := ctl(t, handler, http.MethodPost, "/_ctl/action",
		`{"repo":"hay-kot/hive-desktop","num":58,"action":"merge"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	overlay, ok := store.Get("hay-kot/hive-desktop#58")
	require.True(t, ok)
	require.NotNil(t, overlay.State)
	assert.Equal(t, "merged", *overlay.State)
	require.NotNil(t, overlay.Absent)
	assert.True(t, *overlay.Absent, "a merge must also drop the item from search results")
}

func TestControlRejectsUnknownAction(t *testing.T) {
	handler, _, _ := testControl(t, Config{})
	rec := ctl(t, handler, http.MethodPost, "/_ctl/action",
		`{"repo":"o/r","num":1,"action":"approve"}`)
	// "approve" is deliberately absent: GitHub has no such notification
	// reason, and inventing one would teach a wrong lesson about the app.
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), "unknown action")
}

func TestControlRejectsInvalidMatcher(t *testing.T) {
	handler, _, _ := testControl(t, Config{})
	for _, body := range []string{
		`{"repo":"noslash","num":1,"action":"comment"}`,
		`{"repo":"o/r","num":0,"action":"comment"}`,
		`{"repo":"","num":1,"action":"comment"}`,
	} {
		rec := ctl(t, handler, http.MethodPost, "/_ctl/action", body)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, body)
	}
}

func TestControlRejectsUnknownFields(t *testing.T) {
	handler, _, _ := testControl(t, Config{})
	rec := ctl(t, handler, http.MethodPost, "/_ctl/action",
		`{"repo":"o/r","num":1,"action":"comment","typo":true}`)
	// A typo in a hand-written curl call should fail loudly, not silently do
	// nothing and look like a devserver bug.
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestControlOverlaySetAndClear(t *testing.T) {
	handler, store, _ := testControl(t, Config{})

	rec := ctl(t, handler, http.MethodPost, "/_ctl/overlay",
		`{"repo":"o/r","num":1,"set":{"state":"closed","labels":["bug"]}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	overlay, ok := store.Get("o/r#1")
	require.True(t, ok)
	require.NotNil(t, overlay.Labels)
	assert.Equal(t, []string{"bug"}, *overlay.Labels)

	rec = ctl(t, handler, http.MethodPost, "/_ctl/overlay/clear", `{"repo":"o/r","num":1}`)
	require.Equal(t, http.StatusOK, rec.Code)
	_, ok = store.Get("o/r#1")
	assert.False(t, ok)
}

func TestControlOverlayRejectsInvalidState(t *testing.T) {
	handler, _, _ := testControl(t, Config{})
	rec := ctl(t, handler, http.MethodPost, "/_ctl/overlay",
		`{"repo":"o/r","num":1,"set":{"state":"squashed"}}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), "not one of")
}

func TestControlClearAll(t *testing.T) {
	handler, store, _ := testControl(t, Config{})
	store.Apply("o/r#1", Mutations{State: new("closed")})
	store.Apply("o/r#2", Mutations{State: new("closed")})

	require.Equal(t, http.StatusOK, ctl(t, handler, http.MethodPost, "/_ctl/overlays/clear", `{}`).Code)
	assert.Empty(t, store.Overlays())
}

// TestControlHealthStillSatisfiesTheProbe guards the standby race: enriching
// health with readiness fields must not stop devproxy.Probe decoding the
// Devserver marker, or a standby launch would treat the live server as foreign.
func TestControlHealthStillSatisfiesTheProbe(t *testing.T) {
	handler, _, _ := testControl(t, Config{})

	rec := ctl(t, handler, http.MethodGet, devproxy.HealthPath, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var probe devproxy.Health
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &probe))
	assert.True(t, probe.Devserver)
}

func TestControlStateReportsEverythingTheDashboardNeeds(t *testing.T) {
	handler, store, _ := testControl(t, Config{
		Webhooks: WebhookConfig{
			Targets:  []WebhookTarget{{Name: "local", URL: "http://127.0.0.1:1/hooks/x", Secret: "s"}},
			Payloads: map[string]map[string]any{"pr-opened": {"id": "1"}},
		},
	})
	store.Apply("o/r#1", Mutations{State: new("closed")})

	rec := ctl(t, handler, http.MethodGet, "/_ctl/state", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var view StateView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &view))
	assert.Equal(t, DefaultUpstream, view.Upstream)
	assert.Contains(t, view.Overlays, "o/r#1")
	assert.Empty(t, view.Scenarios, "no scenario is running")
	require.Len(t, view.Targets, 1)
	assert.Equal(t, []string{"pr-opened"}, view.Payloads)
	assert.NotEmpty(t, view.Actions, "the dashboard renders whatever the vocabulary contains")
}

// TestControlStateJSONKeysMatchDashboard pins the wire keys the dashboard's
// JS reads. Decoding back into the Go structs would pass whatever the field
// names are, so it cannot catch a missing json tag — which is exactly how the
// target list first shipped rendering "undefined".
func TestControlStateJSONKeysMatchDashboard(t *testing.T) {
	handler, store, _ := testControl(t, Config{
		Webhooks: WebhookConfig{
			Targets:  []WebhookTarget{{Name: "local", URL: "http://127.0.0.1:1/hooks/x"}},
			Payloads: map[string]map[string]any{"p": {"id": "1"}},
		},
	})
	store.Apply("o/r#1", Mutations{State: new("closed"), Absent: new(true)})

	var raw map[string]any
	require.NoError(t, json.Unmarshal(ctl(t, handler, http.MethodGet, "/_ctl/state", "").Body.Bytes(), &raw))

	for _, key := range []string{
		"upstream", "cacheTTL", "cacheEntries", "stats", "items",
		"overlays", "scenarios", "targets", "payloads", "recent", "pushes", "actions",
	} {
		assert.Contains(t, raw, key)
	}

	targets, ok := raw["targets"].([]any)
	require.True(t, ok)
	require.Len(t, targets, 1)
	assert.Contains(t, targets[0], "name")
	assert.Contains(t, targets[0], "url")
	assert.NotContains(t, targets[0], "secret", "the secret must not be serializable at all")

	overlay, ok := raw["overlays"].(map[string]any)["o/r#1"].(map[string]any)
	require.True(t, ok)
	// Lowercase mutation keys, and untouched fields omitted rather than null.
	assert.Equal(t, "closed", overlay["state"])
	assert.Equal(t, true, overlay["absent"])
	assert.NotContains(t, overlay, "labels")

	action, ok := raw["actions"].([]any)[0].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, action, "name")
	assert.Contains(t, action, "label")
}

func TestControlStateNeverLeaksWebhookSecrets(t *testing.T) {
	handler, _, _ := testControl(t, Config{
		Webhooks: WebhookConfig{Targets: []WebhookTarget{
			{Name: "local", URL: "http://127.0.0.1:1/hooks/x", Secret: "super-secret"},
		}},
	})
	rec := ctl(t, handler, http.MethodGet, "/_ctl/state", "")
	assert.NotContains(t, rec.Body.String(), "super-secret")
}

func TestControlScenarioAppliesStepsInOrder(t *testing.T) {
	handler, store, _ := testControl(t, Config{})

	rec := ctl(t, handler, http.MethodPost, "/_ctl/scenario", `{"steps":[
		{"repo":"o/r","num":1,"action":"review-requested"},
		{"repo":"o/r","num":1,"set":{"state":"merged","absent":true}}
	]}`)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	var accepted struct {
		Scenario string `json:"scenario"`
		Steps    int    `json:"steps"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &accepted))
	assert.Equal(t, "scenario-1", accepted.Scenario)
	assert.Equal(t, 2, accepted.Steps)

	require.Eventually(t, func() bool {
		overlay, ok := store.Get("o/r#1")
		return ok && overlay.State != nil && *overlay.State == "merged"
	}, 2*time.Second, 10*time.Millisecond)

	overlay, _ := store.Get("o/r#1")
	require.NotNil(t, overlay.Reason)
	assert.Equal(t, "review_requested", *overlay.Reason, "later steps must accumulate, not replace")
}

func TestControlScenarioSupportsActionAndSetSteps(t *testing.T) {
	handler, store, _ := testControl(t, Config{})

	rec := ctl(t, handler, http.MethodPost, "/_ctl/scenario",
		`{"steps":[{"repo":"o/r","num":7,"set":{"labels":["ci-failed"]}}]}`)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	require.Eventually(t, func() bool {
		overlay, ok := store.Get("o/r#7")
		return ok && overlay.Labels != nil
	}, 2*time.Second, 10*time.Millisecond)
	overlay, _ := store.Get("o/r#7")
	assert.Equal(t, []string{"ci-failed"}, *overlay.Labels)
}

// TestControlScenarioReportsRunningInState pins that an in-flight scenario is
// visible in /_ctl/state — the only way a caller learns its run is still going
// — and disappears once done. The wait step keeps it in flight across the read.
func TestControlScenarioReportsRunningInState(t *testing.T) {
	handler, _, _ := testControl(t, Config{})

	rec := ctl(t, handler, http.MethodPost, "/_ctl/scenario",
		`{"steps":[{"repo":"o/r","num":1,"action":"comment"},{"wait":"1s"}]}`)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	var view StateView
	require.NoError(t, json.Unmarshal(ctl(t, handler, http.MethodGet, "/_ctl/state", "").Body.Bytes(), &view))
	require.Len(t, view.Scenarios, 1, "a scenario mid-wait must show as running")
	assert.Equal(t, "scenario-1", view.Scenarios[0].Name)
	assert.Equal(t, 2, view.Scenarios[0].Steps)

	require.Eventually(t, func() bool {
		var v StateView
		require.NoError(t, json.Unmarshal(ctl(t, handler, http.MethodGet, "/_ctl/state", "").Body.Bytes(), &v))
		return len(v.Scenarios) == 0
	}, 3*time.Second, 20*time.Millisecond)
}

func TestControlScenarioRejectsBadSteps(t *testing.T) {
	handler, _, _ := testControl(t, Config{})
	for name, body := range map[string]string{
		"no steps":       `{"steps":[]}`,
		"unknown action": `{"steps":[{"repo":"o/r","num":1,"action":"approve"}]}`,
		"action and set": `{"steps":[{"repo":"o/r","num":1,"action":"comment","set":{"state":"open"}}]}`,
		"bad matcher":    `{"steps":[{"repo":"noslash","num":1,"action":"comment"}]}`,
		"no-op step":     `{"steps":[{"repo":"o/r","num":1}]}`,
		"bad wait":       `{"steps":[{"wait":"soon"}]}`,
		"bad mutation":   `{"steps":[{"repo":"o/r","num":1,"set":{"state":"squashed"}}]}`,
	} {
		rec := ctl(t, handler, http.MethodPost, "/_ctl/scenario", body)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, name)
	}

	// A typo'd key is a body the decoder rejects, not a validation failure.
	rec := ctl(t, handler, http.MethodPost, "/_ctl/scenario",
		`{"steps":[{"repo":"o/r","num":1,"action":"comment","typo":true}]}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ── Pusher ───────────────────────────────────────────────────────────────────

func TestPusherDeliversPayloadWithSecret(t *testing.T) {
	var (
		mu        sync.Mutex
		secret    string
		body      map[string]any
		decodeErr error
	)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		secret = r.Header.Get(secretHeader)
		decodeErr = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"delivered":1}`)) //nolint:errcheck // test server
	}))
	defer target.Close()

	handler, _, pusher := testControl(t, Config{Webhooks: WebhookConfig{
		Targets:  []WebhookTarget{{Name: "local", URL: target.URL, Secret: "s3cret"}},
		Payloads: map[string]map[string]any{"pr-opened": {"id": "pr-1", "title": "Add retry"}},
	}})

	rec := ctl(t, handler, http.MethodPost, "/_ctl/webhooks/push",
		`{"target":"local","payload":"pr-opened","overrides":{"state":"resolved"}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	mu.Lock()
	defer mu.Unlock()
	require.NoError(t, decodeErr)
	// The header the desktop's webhook listener authenticates on.
	assert.Equal(t, "s3cret", secret)
	assert.Equal(t, "pr-1", body["id"])
	assert.Equal(t, "Add retry", body["title"])
	assert.Equal(t, "resolved", body["state"], "overrides must merge over the named payload")

	require.Len(t, pusher.Recent(), 1)
	assert.Equal(t, http.StatusAccepted, pusher.Recent()[0].Status)
}

func TestPusherOverridesDoNotMutateStoredPayload(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer target.Close()

	handler, _, pusher := testControl(t, Config{Webhooks: WebhookConfig{
		Targets:  []WebhookTarget{{Name: "local", URL: target.URL}},
		Payloads: map[string]map[string]any{"p": {"id": "1"}},
	}})

	ctl(t, handler, http.MethodPost, "/_ctl/webhooks/push", `{"target":"local","payload":"p","overrides":{"state":"resolved"}}`)

	// A push must not permanently rewrite the configured payload, or the
	// second push of the same name would be a different event.
	stored, ok := pusher.Payload("p")
	require.True(t, ok)
	assert.NotContains(t, stored, "state")
}

func TestPusherInlineBody(t *testing.T) {
	var (
		body      map[string]any
		decodeErr error
	)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeErr = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer target.Close()

	handler, _, _ := testControl(t, Config{Webhooks: WebhookConfig{
		Targets: []WebhookTarget{{Name: "local", URL: target.URL}},
	}})

	rec := ctl(t, handler, http.MethodPost, "/_ctl/webhooks/push",
		`{"target":"local","body":{"id":"x","title":"Inline"}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, decodeErr)
	assert.Equal(t, "Inline", body["title"])
}

// TestPushInlineTargetDelivers covers a target supplied by URL rather than by
// name — the only way to reach a desktop instance, whose webhook port is random
// per install so no target can be committed to config.
func TestPushInlineTargetDelivers(t *testing.T) {
	var (
		mu     sync.Mutex
		secret string
		body   map[string]any
	)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		secret = r.Header.Get(secretHeader)
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer target.Close()

	handler, _, _ := testControl(t, Config{})
	rec := ctl(t, handler, http.MethodPost, "/_ctl/webhooks/push",
		`{"target":{"url":"`+target.URL+`","secret":"s3cret"},"body":{"id":"x","title":"Inline"}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "s3cret", secret)
	assert.Equal(t, "Inline", body["title"])
}

func TestPushRejectsNonLoopbackInlineTarget(t *testing.T) {
	handler, _, _ := testControl(t, Config{})
	rec := ctl(t, handler, http.MethodPost, "/_ctl/webhooks/push",
		`{"target":{"url":"http://example.com/hooks/x"},"body":{"id":"x"}}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), "loopback")
}

func TestPushRequiresExactlyOneTargetForm(t *testing.T) {
	handler, _, _ := testControl(t, Config{Webhooks: WebhookConfig{
		Payloads: map[string]map[string]any{"p": {"id": "1"}},
	}})
	// Neither a name nor a url.
	rec := ctl(t, handler, http.MethodPost, "/_ctl/webhooks/push", `{"payload":"p"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), "target")
}

func TestPusherRequiresExactlyOnePayloadSource(t *testing.T) {
	handler, _, _ := testControl(t, Config{Webhooks: WebhookConfig{
		Targets:  []WebhookTarget{{Name: "local", URL: "http://127.0.0.1:1/hooks/x"}},
		Payloads: map[string]map[string]any{"p": {"id": "1"}},
	}})

	for _, body := range []string{
		`{"target":"local"}`,
		`{"target":"local","payload":"p","body":{"id":"1"}}`,
	} {
		rec := ctl(t, handler, http.MethodPost, "/_ctl/webhooks/push", body)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, body)
	}
}

func TestPusherReportsTargetFailure(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no webhook endpoint at this path", http.StatusNotFound)
	}))
	defer target.Close()

	handler, _, pusher := testControl(t, Config{Webhooks: WebhookConfig{
		Targets:  []WebhookTarget{{Name: "local", URL: target.URL}},
		Payloads: map[string]map[string]any{"p": {"id": "1"}},
	}})

	rec := ctl(t, handler, http.MethodPost, "/_ctl/webhooks/push", `{"target":"local","payload":"p"}`)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	// The most likely real failure is a path that matches no sources.webhook
	// node, so the target's own message has to reach the author.
	assert.Contains(t, rec.Body.String(), "no webhook endpoint")
	require.Len(t, pusher.Recent(), 1)
	assert.NotEmpty(t, pusher.Recent()[0].Error)
}

func TestPusherRejectsUnknownTargetAndPayload(t *testing.T) {
	handler, _, _ := testControl(t, Config{Webhooks: WebhookConfig{
		Targets:  []WebhookTarget{{Name: "local", URL: "http://127.0.0.1:1/hooks/x"}},
		Payloads: map[string]map[string]any{"p": {"id": "1"}},
	}})

	rec := ctl(t, handler, http.MethodPost, "/_ctl/webhooks/push", `{"target":"nope","payload":"p"}`)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "no webhook target")

	rec = ctl(t, handler, http.MethodPost, "/_ctl/webhooks/push", `{"target":"local","payload":"nope"}`)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "no payload named")
}

// ── Config ───────────────────────────────────────────────────────────────────

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "devserver.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// TestDefaultCachePathIsIsolatedFromAppState pins the cache to the XDG cache
// dir and, specifically, off the desktop's data root. ADR desktop-configuration gives every
// worktree an isolated instance that the dev:reset task exists to delete, so
// deriving from it would give every worktree a different cache and discard it
// on reset — losing the sharing and persistence the cache exists for.
func TestDefaultCachePathIsIsolatedFromAppState(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/cache-home")
	t.Setenv("XDG_DATA_HOME", "/data-home")
	t.Setenv("HIVE_DESKTOP_DATA_DIR", "/worktree-local-data")

	path := DefaultCachePath()
	assert.Equal(t, filepath.Join("/cache-home", "hive", "devserver", "cache.db"), path)
	assert.NotContains(t, path, "worktree-local-data", "the cache must not follow the app's data dir")
	assert.NotContains(t, path, "data-home", "nor the app's data home")

	// Falls back to ~/.cache, never to the data dir.
	t.Setenv("XDG_CACHE_HOME", "")
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".cache", "hive", "devserver", "cache.db"), DefaultCachePath())
}

func TestResolveConfigPath(t *testing.T) {
	existing := writeConfig(t, "listen: 127.0.0.1:1\n")

	got, err := ResolveConfigPath(existing)
	require.NoError(t, err)
	assert.Equal(t, existing, got)

	// An explicit path that does not exist must fail rather than silently
	// degrading to a bare proxy — that would look like the config was ignored.
	_, err = ResolveConfigPath(filepath.Join(t.TempDir(), "absent.yaml"))
	require.Error(t, err)

	// No flag falls back to the checked-in repo config. There is deliberately
	// no per-user location: development config is versioned with the code it
	// simulates, not hidden in a home directory.
	got, err = ResolveConfigPath("")
	require.NoError(t, err)
	assert.Equal(t, devproxy.RepoConfigPath, got)
}

// TestRepoConfigIsValidAndInert guards the checked-in development config that
// `mise run desktop:devserver` passes. It must parse, and it must declare no overlays:
// a config that rewrote data the moment devserver started would make every
// subsequent bug suspect.
func TestRepoConfigIsValidAndInert(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join("..", "..", "..", devproxy.RepoConfigPath))
	require.NoError(t, err, "the shipped config must parse")

	assert.Empty(t, cfg.Overlays, "the shipped config must not seed overlays")
	assert.NotEmpty(t, cfg.Webhooks.Payloads, "it should give the pusher something to send")
	// Targets cannot be shipped: the desktop's webhook port is random per
	// install, so any committed URL would just fail.
	assert.Empty(t, cfg.Webhooks.Targets)
}

func TestLoadConfigMissingFileYieldsWorkingDefaults(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "absent.yaml"))
	require.NoError(t, err)
	// No config at all still has to give the "just stop burning my rate
	// limit" behaviour, which is the primary use.
	assert.Equal(t, DefaultListen, cfg.Listen)
	assert.Equal(t, DefaultUpstream, cfg.Upstream)
	assert.Equal(t, DefaultTTL, cfg.Cache.TTL)
	assert.NotEmpty(t, cfg.Cache.Path)
}

func TestBuildStepsAllowsWaitOnly(t *testing.T) {
	steps, err := buildSteps([]stepInput{{Wait: "1s"}})
	require.NoError(t, err)
	require.Len(t, steps, 1)
	assert.Equal(t, time.Second, steps[0].Wait)
	assert.True(t, steps[0].Set.Empty(), "a wait-only step carries no mutation")
}

func TestLoadConfigParsesFullFile(t *testing.T) {
	cfg, err := LoadConfig(writeConfig(t, `
listen: 127.0.0.1:9999
upstream: https://api.github.com/
cache:
  ttl: 30s
overlays:
  - match: {repo: hay-kot/hive-desktop, num: 58}
    set:
      state: open
      reason: approval_requested
      labels: [needs-review]
webhooks:
  targets:
    - name: local
      url: http://127.0.0.1:24681/hooks/devserver
      secret: dev
  payloads:
    pr-opened:
      id: pr-1
      title: Add retry
`))
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:9999", cfg.Listen)
	assert.Equal(t, "https://api.github.com", cfg.Upstream, "a trailing slash must be trimmed")
	assert.Equal(t, 30*time.Second, cfg.Cache.TTL)

	require.Len(t, cfg.Overlays, 1)
	require.NotNil(t, cfg.Overlays[0].Set.Labels)
	assert.Equal(t, []string{"needs-review"}, *cfg.Overlays[0].Set.Labels)
	assert.Equal(t, "hay-kot/hive-desktop#58", cfg.Overlays[0].Match.Key())

	require.Len(t, cfg.Webhooks.Targets, 1)
	assert.Equal(t, "dev", cfg.Webhooks.Targets[0].Secret)
	assert.Equal(t, "pr-1", cfg.Webhooks.Payloads["pr-opened"]["id"])
}

func TestLoadConfigRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"bad overlay repo":    "overlays:\n  - match: {repo: nope, num: 1}\n    set: {state: open}\n",
		"zero overlay num":    "overlays:\n  - match: {repo: o/r, num: 0}\n    set: {state: open}\n",
		"unknown state":       "overlays:\n  - match: {repo: o/r, num: 1}\n    set: {state: squashed}\n",
		"target missing name": "webhooks:\n  targets:\n    - url: http://127.0.0.1:1/x\n",
		"target bad url":      "webhooks:\n  targets:\n    - {name: a, url: ftp://x}\n",
		"duplicate target":    "webhooks:\n  targets:\n    - {name: a, url: 'http://127.0.0.1:1/x'}\n    - {name: a, url: 'http://127.0.0.1:2/x'}\n",
		"malformed yaml":      "listen: [unclosed\n",
	}
	for name, body := range cases {
		_, err := LoadConfig(writeConfig(t, body))
		assert.Error(t, err, name)
	}
}
