// Package docstest checks that a registry and its embedded docs/<type>.md files
// name the same types. Some prompts are built from these docs, so a new type with
// no doc would show as a gap, and a removed type must not leave a doc behind.
package docstest

import (
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertCoverTypes checks that each type in types has a doc that opens with an H1,
// and that docs holds no file for a type outside types. kind names the types in
// failure messages.
func AssertCoverTypes(t *testing.T, kind string, types []string, doc func(string) (string, error), docs fs.ReadDirFS) {
	t.Helper()
	for _, name := range types {
		content, err := doc(name)
		require.NoErrorf(t, err, "%s type %q has no docs/%s.md", kind, name, name)
		assert.NotEmptyf(t, strings.TrimSpace(content), "docs/%s.md is empty", name)
		assert.Truef(t, strings.HasPrefix(strings.TrimSpace(content), "# "),
			"docs/%s.md must open with an H1 naming the %s type", name, kind)
	}

	entries, err := docs.ReadDir("docs")
	require.NoError(t, err)
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".md")
		assert.Truef(t, slices.Contains(types, name),
			"docs/%s documents %q, which is not a registered %s type", entry.Name(), name, kind)
	}
}
