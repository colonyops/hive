package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

func TestSearchIndexesHTMLText(t *testing.T) {
	e := newEnv(t, devPolicy())
	p := principalFor(t, e, "dev@example.com")
	mustPut(t, e, p, drive.Shared, "page.html", `<!doctype html><html><head><style>.zebra{color:red}</style>
<script>var secret = "hidden";</script></head>
<body><!-- note --><div class="x">hello &amp; welcome</div></body></html>`)

	assert.Equal(t, []string{"page.html"}, searchPaths(t, e, p, "hello", nil))
	assert.Equal(t, []string{"page.html"}, searchPaths(t, e, p, "welcome", nil))
	for _, q := range []string{"div", "class", "zebra", "secret", "note", "amp"} {
		assert.Empty(t, searchPaths(t, e, p, q, nil), q)
	}

	hits, err := e.docs.Search(t.Context(), p, "hello", nil, 10)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.NotContains(t, hits[0].Snippet, "<")
	assert.Contains(t, hits[0].Snippet, driveapi.SnippetMatchStart+"hello"+driveapi.SnippetMatchEnd)
}
