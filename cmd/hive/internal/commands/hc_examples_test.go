package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/schema"
)

// TestHCCreateExamplesValidate keeps every bulk-create example an agent can
// read honest: each JSON object in the command's help text, the hc skill, and
// the docs page must pass the same checks `hive hc create` applies.
func TestHCCreateExamplesValidate(t *testing.T) {
	sources := map[string]string{
		"help text": (&HoneycombCmd{}).createCmd().Description,
	}
	for name, rel := range map[string]string{
		"skill": "claude-plugin/hive/skills/hc/SKILL.md",
		"docs":  "docs/docs/cli/getting-started/task-tracking.md",
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", rel))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		sources[name] = string(data)
	}

	total := 0
	for name, text := range sources {
		examples := jsonObjects(text)
		if len(examples) == 0 {
			t.Errorf("%s: no bulk-create example found; extractor or source changed", name)
		}
		for i, raw := range examples {
			total++
			var doc any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("%s example %d: %v", name, i, err)
			}
			if err := schema.Validate("hc.tree", doc); err != nil {
				t.Errorf("%s example %d fails the schema: %v\n%s", name, i, err, raw)
				continue
			}
			var input hc.CreateInput
			if err := json.Unmarshal(raw, &input); err != nil {
				t.Fatalf("%s example %d: %v", name, i, err)
			}
			if err := hive.ValidateCreateInput(input); err != nil {
				t.Errorf("%s example %d fails the tree rules: %v\n%s", name, i, err, raw)
			}
		}
	}
	if total < 5 {
		t.Errorf("only %d examples found across all sources; the extractor is probably broken", total)
	}
}

// jsonObjects returns every balanced {...} span in text that parses as a JSON
// object with "title" and "type" and without "id": the shape of a bulk-create
// document. Output samples carry an id, and prose braces fail to parse, so
// both are skipped.
func jsonObjects(text string) []json.RawMessage {
	var out []json.RawMessage
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		end := matchingBrace(text, i)
		if end < 0 {
			continue
		}
		candidate := []byte(text[i : end+1])
		var obj map[string]any
		if err := json.Unmarshal(candidate, &obj); err != nil {
			continue
		}
		if _, ok := obj["title"]; !ok {
			continue
		}
		if _, ok := obj["type"]; !ok {
			continue
		}
		if _, ok := obj["id"]; ok {
			continue
		}
		out = append(out, json.RawMessage(candidate))
		i = end
	}
	return out
}

func matchingBrace(text string, start int) int {
	depth := 0
	inString := false
	for i := start; i < len(text); i++ {
		c := text[i]
		if inString {
			switch c {
			case '\\':
				i++
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
