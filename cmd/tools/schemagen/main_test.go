package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectDropsGeneratorFields(t *testing.T) {
	entries := []entry{{
		ID: "hc.tree", Title: "t", Description: "d", Dir: "hc",
		Definition: "#CreateInput", GoOut: "internal/core/hc/create_input.gen.go", Doc: "/cli/",
	}}
	data, err := json.Marshal(project(entries))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, absent := range []string{"dir", "definition", "go_out"} {
		if strings.Contains(got, `"`+absent+`"`) {
			t.Errorf("catalog.json carries generator field %q: %s", absent, got)
		}
	}
	for _, present := range []string{`"id":"hc.tree"`, `"doc":"/cli/"`} {
		if !strings.Contains(got, present) {
			t.Errorf("catalog.json lacks %s: %s", present, got)
		}
	}
}
