package drive

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMarkdownFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		head    string
		want    Frontmatter
		present bool
	}{
		{"none", "# Title\n\nbody\n", nil, false},
		{"empty head", "", nil, false},
		{"fence not on the first line", "\n---\ntype: plan\n---\n", nil, false},
		{"rule later in the body", "# Title\n\n---\n", nil, false},
		{"present", "---\ntype: plan\ntags: [desktop, cli]\ndraft: true\nscore: 9.5\n---\n\n# Title\n", Frontmatter{
			"type": "plan", "tags": []any{"desktop", "cli"}, "draft": true, "score": 9.5,
		}, true},
		{"closing fence at end of input", "---\ntype: plan\n---", Frontmatter{"type": "plan"}, true},
		{"closing fence with trailing blanks", "---\na: 1\n--- \n", Frontmatter{"a": 1}, true},
		{"closing fence with a trailing tab at end of input", "---\na: 1\n---\t", Frontmatter{"a": 1}, true},
		{"empty block", "---\n---\n# Title\n", Frontmatter{}, true},
		{"crlf", "---\r\ntype: plan\r\n---\r\nbody\r\n", Frontmatter{"type": "plan"}, true},
		{"bom", "\xef\xbb\xbf---\ntype: plan\n---\n", Frontmatter{"type": "plan"}, true},
		{"dates stay text", "---\ndate: 2026-10-11\nat: 2026-10-11T10:00:00Z\n---\n", Frontmatter{
			"date": "2026-10-11", "at": "2026-10-11T10:00:00Z",
		}, true},
		{"nested values kept", "---\nupdates:\n  - date: 2026-10-11\n    note: initial\n---\n", Frontmatter{
			"updates": []any{map[string]any{"date": "2026-10-11", "note": "initial"}},
		}, true},
		{"a line that only starts like the fence is not the fence", "---\ntitle: a\n---b: 1\n---\n", Frontmatter{
			"title": "a", "---b": 1,
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, present, err := ParseMarkdownFrontmatter([]byte(tt.head))
			require.NoError(t, err)
			assert.Equal(t, tt.present, present)
			assert.Equal(t, tt.want, fm)
		})
	}
}

func TestParseMarkdownFrontmatterErrors(t *testing.T) {
	for name, head := range map[string]string{
		"unterminated":       "---\ntype: plan\n",
		"cut short":          "---\ntype: plan\n--",
		"only opening fence": "---\n",
		"bad yaml":           "---\ntype: [plan\n---\n",
		"not a mapping":      "---\njust text\n---\n",
	} {
		t.Run(name, func(t *testing.T) {
			fm, present, err := ParseMarkdownFrontmatter([]byte(head))
			require.ErrorIs(t, err, ErrInvalidFrontmatter)
			assert.False(t, present)
			assert.Nil(t, fm)
		})
	}
}

func TestValidateFrontmatter(t *testing.T) {
	manyKeys := Frontmatter{}
	for i := range maxFrontmatterKeys + 1 {
		manyKeys[strings.Repeat("k", i+1)] = i
	}
	longList := make([]any, maxFrontmatterList+1)
	for i := range longList {
		longList[i] = i
	}
	tooLarge := Frontmatter{}
	for i := range 20 {
		tooLarge["k"+strings.Repeat("x", i)] = strings.Repeat("y", maxFrontmatterText)
	}

	for name, fm := range map[string]Frontmatter{
		"reserved created": {"created_at": "yesterday"},
		"reserved updated": {"UPDATED_AT": "today"},
		"bad key":          {"review status": "ready"},
		"leading dot key":  {".hidden": 1},
		"long key":         {strings.Repeat("k", maxFrontmatterKeyBytes+1): 1},
		"nested object":    {"owner": map[string]any{"name": "Ada"}},
		"nested list":      {"matrix": []any{[]any{"x"}}},
		"long text":        {"notes": strings.Repeat("x", maxFrontmatterText+1)},
		"long list":        {"items": longList},
		"too many keys":    manyKeys,
		"too large":        tooLarge,
	} {
		require.ErrorIs(t, ValidateFrontmatter(fm), ErrInvalidFrontmatter, name)
	}
	assert.NoError(t, ValidateFrontmatter(Frontmatter{
		"tags": []any{"release", "desktop"}, "owner": "Ada", "ready": true, "score": float64(9.5), "note": nil, "n": int64(3),
	}))
	assert.NoError(t, ValidateFrontmatter(nil))
}
