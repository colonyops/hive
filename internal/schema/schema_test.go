package schema

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/hay-kot/criterio"
	"gopkg.in/yaml.v3"
)

func TestEveryCatalogEntryHasACompilableSchema(t *testing.T) {
	entries := Catalog()
	if len(entries) == 0 {
		t.Fatal("catalog is empty")
	}
	for _, e := range entries {
		raw, err := JSON(e.ID)
		if err != nil {
			t.Fatalf("%s: %v", e.ID, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s: not JSON: %v", e.ID, err)
		}
		if _, err := compiledSchema(e.ID); err != nil {
			t.Fatalf("%s: %v", e.ID, err)
		}
		if _, ok := Lookup(e.ID); !ok {
			t.Errorf("Lookup(%q) missed its own entry", e.ID)
		}
	}
}

func TestValidateHCTree(t *testing.T) {
	good := `{"title":"Auth","type":"epic","children":[
		{"ref":"jwt","title":"JWT middleware","type":"task"},
		{"title":"Login","type":"task","blockers":["jwt"]}]}`
	if err := Validate("hc.tree", decode(t, good)); err != nil {
		t.Fatalf("valid tree rejected: %v", err)
	}

	bad := `{"title":"","type":"tsak","typo":1,"children":[{"title":"x","type":"task","ref":"Bad Ref"}]}`
	err := Validate("hc.tree", decode(t, bad))
	var fieldErrs criterio.FieldErrors
	if !errors.As(err, &fieldErrs) {
		t.Fatalf("want criterio.FieldErrors, got %T: %v", err, err)
	}
	got := map[string]string{}
	for _, fe := range fieldErrs {
		got[fe.Field] = fe.Err.Error()
	}
	for _, field := range []string{"title", "type", "typo", "children[0].ref"} {
		if _, ok := got[field]; !ok {
			t.Errorf("no error for %q; got %v", field, got)
		}
	}
	if got["typo"] != "unknown key" {
		t.Errorf("typo: want %q, got %q", "unknown key", got["typo"])
	}
}

func TestValidateUnknownID(t *testing.T) {
	if err := Validate("nope.nope", map[string]any{}); err == nil {
		t.Fatal("unknown id accepted")
	}
}

func TestNormalizeYAML(t *testing.T) {
	var v any
	if err := yaml.Unmarshal([]byte("title: x\ntype: epic\nchildren:\n  - title: y\n    type: task\n"), &v); err != nil {
		t.Fatal(err)
	}
	n, err := Normalize(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate("hc.tree", n); err != nil {
		t.Fatalf("normalized yaml rejected: %v", err)
	}
}

func TestFieldPath(t *testing.T) {
	cases := map[string][]string{
		"":                        nil,
		"title":                   {"title"},
		"children[1].blockers[0]": {"children", "1", "blockers", "0"},
		"tui.theme":               {"tui", "theme"},
	}
	for want, segments := range cases {
		if got := fieldPath(segments); got != want {
			t.Errorf("fieldPath(%v) = %q, want %q", segments, got, want)
		}
	}
}

func TestPublishedURL(t *testing.T) {
	if got := PublishedURL("hc.tree"); got != "https://hivedesktop.com/schemas/hc.tree.schema.json" {
		t.Errorf("PublishedURL = %q", got)
	}
}

func decode(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}
