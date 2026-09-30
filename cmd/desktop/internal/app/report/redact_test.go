package report

import "testing"

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", v)
	}
	return m
}

func asSlice(t *testing.T, v any) []any {
	t.Helper()
	s, ok := v.([]any)
	if !ok {
		t.Fatalf("expected slice, got %T", v)
	}
	return s
}

func TestRedactSecretKeys(t *testing.T) {
	in := map[string]any{
		"path":          "/hook",
		"secret":        "keepout",
		"api_key":       "keepout",
		"client_secret": "keepout",
		"keybindings":   map[string]any{"save": "ctrl+s"},
	}
	out := asMap(t, Redact(in))

	for _, k := range []string{"secret", "api_key", "client_secret"} {
		if out[k] != redacted {
			t.Errorf("%s not redacted: %v", k, out[k])
		}
	}
	if out["path"] != "/hook" {
		t.Errorf("non-secret path was altered: %v", out["path"])
	}
	if _, ok := out["keybindings"].(map[string]any); !ok {
		t.Error("keybindings map was dropped (false-positive key match)")
	}
}

func TestRedactCarrierMaps(t *testing.T) {
	in := map[string]any{
		"env": map[string]any{
			"HARMLESS":   "value-with-no-pattern",
			"DEPLOY_KEY": "sk_live_whatever",
		},
	}
	env := asMap(t, asMap(t, Redact(in))["env"])
	for k, v := range env {
		if v != redacted {
			t.Errorf("env[%s] not redacted: %v", k, v)
		}
	}
}

func TestRedactCarrierListForm(t *testing.T) {
	in := map[string]any{
		"env": []any{"HARMLESS=value-with-no-pattern", "DEPLOY_KEY=hunter2"},
	}
	env := asSlice(t, asMap(t, Redact(in))["env"])
	for i, v := range env {
		if v != redacted {
			t.Errorf("env[%d] not redacted: %v", i, v)
		}
	}
}

func TestRedactValuePatterns(t *testing.T) {
	cases := map[string]bool{
		"ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA":   true,
		"github_pat_ABCDEFGHIJ1234567890abcdefghij":  true,
		"Authorization: Bearer abcdef0123456789ABCD": true,
		"just some normal log text":                  false,
	}
	for in, shouldChange := range cases {
		got := scrubText(in)
		if shouldChange && got == in {
			t.Errorf("expected scrub of %q", in)
		}
		if !shouldChange && got != in {
			t.Errorf("unexpected scrub of %q -> %q", in, got)
		}
	}
}

func TestRedactNestedStructures(t *testing.T) {
	in := map[string]any{
		"nodes": []any{
			map[string]any{"id": "a", "token": "leak"},
			map[string]any{"id": "b", "value": "ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		},
	}
	nodes := asSlice(t, asMap(t, Redact(in))["nodes"])
	first := asMap(t, nodes[0])
	second := asMap(t, nodes[1])

	if first["token"] != redacted {
		t.Error("nested secret key not redacted")
	}
	if second["value"] == "ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" {
		t.Error("nested token value not scrubbed")
	}
	if first["id"] != "a" {
		t.Error("non-secret nested value altered")
	}
}
