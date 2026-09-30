package rss

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parse(t *testing.T, document string) []Entry {
	t.Helper()
	feed, err := gofeed.NewParser().Parse(strings.NewReader(document))
	require.NoError(t, err)
	return entriesOf(feed)
}

func decode(t *testing.T, entry Entry) payload {
	t.Helper()
	var out payload
	require.NoError(t, json.Unmarshal(entry.Payload, &out))
	return out
}

const rss2 = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <title>Example Blog</title>
  <item>
    <title>Second post</title>
    <link>https://example.com/2</link>
    <guid isPermaLink="false">tag:example.com,2026:2</guid>
    <pubDate>Tue, 10 Feb 2026 09:00:00 +0000</pubDate>
    <description>&lt;p&gt;Newer &amp;amp; better&lt;/p&gt;</description>
    <category>go</category>
  </item>
  <item>
    <title>First post</title>
    <link>https://example.com/1</link>
    <guid isPermaLink="false">tag:example.com,2026:1</guid>
    <pubDate>Mon, 09 Feb 2026 09:00:00 +0000</pubDate>
  </item>
</channel></rss>`

func TestEntriesOfMapsRSSOntoTheCanonicalContract(t *testing.T) {
	t.Parallel()

	entries := parse(t, rss2)
	require.Len(t, entries, 2)

	assert.Equal(t, "tag:example.com,2026:2", entries[0].Key, "the guid is the identity a feed publishes for exactly this")

	got := decode(t, entries[0])
	assert.Equal(t, ItemKind, got.Kind)
	assert.Equal(t, "Second post", got.Title)
	assert.Equal(t, "https://example.com/2", got.URL)
	assert.Equal(t, "Example Blog", got.Repo, "the container line is the feed, not the entry")
	assert.Equal(t, "Newer & better", got.Body, "markup is stripped and entities resolved")
	assert.Equal(t, []string{"go"}, got.Labels)
	assert.Equal(t, "2026-02-10T09:00:00Z", got.Published)
}

// A feed entry has no lifecycle, so inventing a state here would archive items
// on a word the publisher never meant that way.
func TestEntriesCarryNoState(t *testing.T) {
	t.Parallel()

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(parse(t, rss2)[0].Payload, &raw))
	assert.NotContains(t, raw, "state")
}

func TestEntriesOfReadsAtom(t *testing.T) {
	t.Parallel()

	entries := parse(t, `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Releases</title>
  <entry>
    <id>tag:github.com,2008:Repository/1/v2</id>
    <title>v2.0.0</title>
    <link rel="alternate" href="https://example.com/v2"/>
    <updated>2026-02-11T10:00:00Z</updated>
    <author><name>octocat</name></author>
    <content type="html">&lt;p&gt;Notes&lt;/p&gt;</content>
  </entry>
</feed>`)
	require.Len(t, entries, 1)

	got := decode(t, entries[0])
	assert.Equal(t, "tag:github.com,2008:Repository/1/v2", entries[0].Key)
	assert.Equal(t, "v2.0.0", got.Title)
	assert.Equal(t, "https://example.com/v2", got.URL)
	assert.Equal(t, "octocat", got.Author)
	assert.Equal(t, "Notes", got.Body, "an entry with no summary falls back to its content")
	assert.Equal(t, "2026-02-11T10:00:00Z", got.Updated)
}

// The url field takes any of the three, and the docs say so, so the third one
// is held to that here rather than assumed from the parser's feature list.
func TestEntriesOfReadsJSONFeed(t *testing.T) {
	t.Parallel()

	entries := parse(t, `{
	  "version": "https://jsonfeed.org/version/1.1",
	  "title": "Example Blog",
	  "items": [
	    {
	      "id": "https://example.com/3",
	      "url": "https://example.com/3",
	      "title": "Third post",
	      "summary": "A summary",
	      "date_published": "2026-02-12T09:00:00Z",
	      "tags": ["go"]
	    }
	  ]
	}`)
	require.Len(t, entries, 1)

	got := decode(t, entries[0])
	assert.Equal(t, "https://example.com/3", entries[0].Key)
	assert.Equal(t, "Third post", got.Title)
	assert.Equal(t, "Example Blog", got.Repo)
	assert.Equal(t, "A summary", got.Body)
	assert.Equal(t, "2026-02-12T09:00:00Z", got.Published)
}

// The window is newest first regardless of the order a feed lists its entries
// in, because `limit` cuts the tail and the tail has to be the old end.
func TestEntriesOfOrdersNewestFirst(t *testing.T) {
	t.Parallel()

	entries := parse(t, `<rss version="2.0"><channel><title>t</title>
  <item><guid>old</guid><title>Old</title><pubDate>Mon, 09 Feb 2026 09:00:00 +0000</pubDate></item>
  <item><guid>new</guid><title>New</title><pubDate>Wed, 11 Feb 2026 09:00:00 +0000</pubDate></item>
  <item><guid>mid</guid><title>Mid</title><pubDate>Tue, 10 Feb 2026 09:00:00 +0000</pubDate></item>
</channel></rss>`)

	keys := make([]string, len(entries))
	for i, entry := range entries {
		keys[i] = entry.Key
	}
	assert.Equal(t, []string{"new", "mid", "old"}, keys)
}

// An undated entry has nothing to sort on, so it keeps the order the feed
// published it in and sorts behind everything that does carry a date.
func TestEntriesOfSortsUndatedEntriesLastInFeedOrder(t *testing.T) {
	t.Parallel()

	entries := parse(t, `<rss version="2.0"><channel><title>t</title>
  <item><guid>undated-a</guid><title>A</title></item>
  <item><guid>dated</guid><title>D</title><pubDate>Mon, 09 Feb 2026 09:00:00 +0000</pubDate></item>
  <item><guid>undated-b</guid><title>B</title></item>
</channel></rss>`)

	keys := make([]string, len(entries))
	for i, entry := range entries {
		keys[i] = entry.Key
	}
	assert.Equal(t, []string{"dated", "undated-a", "undated-b"}, keys)
}

// A reused guid is the publisher's bug. Dropping the duplicate keeps the other
// entries, where failing would lose the whole window over one bad row.
func TestEntriesOfDropsDuplicateKeys(t *testing.T) {
	t.Parallel()

	entries := parse(t, `<rss version="2.0"><channel><title>t</title>
  <item><guid>same</guid><title>Older</title><pubDate>Mon, 09 Feb 2026 09:00:00 +0000</pubDate></item>
  <item><guid>same</guid><title>Newer</title><pubDate>Tue, 10 Feb 2026 09:00:00 +0000</pubDate></item>
</channel></rss>`)

	require.Len(t, entries, 1)
	assert.Equal(t, "Newer", decode(t, entries[0]).Title, "the newer entry wins")
}

func TestEntryKeyFallsBackToTheLink(t *testing.T) {
	t.Parallel()

	entries := parse(t, `<rss version="2.0"><channel><title>t</title>
  <item><title>No guid</title><link>https://example.com/post</link></item>
</channel></rss>`)

	require.Len(t, entries, 1)
	assert.Equal(t, "https://example.com/post", entries[0].Key)
}

// With neither a guid nor a link, identity is a digest of what is left. It is
// stable across fetches, which is the property that matters; an edited title
// making a new item is the cost of a feed that publishes no ids.
func TestEntryKeyFallsBackToADigest(t *testing.T) {
	t.Parallel()

	document := `<rss version="2.0"><channel><title>t</title>
  <item><title>Bare</title><pubDate>Mon, 09 Feb 2026 09:00:00 +0000</pubDate></item>
</channel></rss>`

	first, second := parse(t, document), parse(t, document)
	require.Len(t, first, 1)
	assert.NotEmpty(t, first[0].Key)
	assert.Equal(t, first[0].Key, second[0].Key)
}

func TestEntriesOfSkipsAnEntryWithNothingToShow(t *testing.T) {
	t.Parallel()

	entries := parse(t, `<rss version="2.0"><channel><title>t</title>
  <item><guid>ghost</guid></item>
  <item><guid>real</guid><title>Real</title></item>
</channel></rss>`)

	require.Len(t, entries, 1)
	assert.Equal(t, "real", entries[0].Key)
}

func TestRenderTextCollapsesAndCutsAtMaxRunes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "a b c", renderText("  a\n b\tc  ", false, 100), "whitespace collapses")
	assert.Empty(t, renderText("   ", false, 100))

	long := renderText(strings.Repeat("é", 50), false, 10)
	assert.Equal(t, strings.Repeat("é", 10)+"…", long, "the cut lands on a rune boundary")
}

// A block boundary is a word boundary. A tag stripper runs these together into
// one word, which is how a summary of short paragraphs became unreadable.
func TestRenderTextSeparatesBlocks(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "one two", renderText("<p>one</p><p>two</p>", false, 100))
	assert.Equal(t, "one two", renderText("one<br>two", false, 100))
}

// Some feeds put nothing in a summary but a link, so discarding the href
// discards the summary. The body is markdown, so the link survives as one.
func TestRenderTextKeepsLinkTargetsInABody(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		"[Comments](https://news.ycombinator.com/item?id=1)",
		renderText(`<a href="https://news.ycombinator.com/item?id=1">Comments</a>`, true, 400))

	assert.Equal(t, "read [the post](https://example.com/p) now",
		renderText(`read <a href="https://example.com/p">the post</a> now`, true, 400))
}

// A title is not markdown, so it keeps the anchor text and nothing else.
func TestRenderTextDropsLinksFromATitle(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Comments", renderText(`<a href="https://example.com">Comments</a>`, false, 400))
}

// "[https://x](https://x)" is noise. Feeds that print the URL as the link text
// (hnrss does) are the common case, not the exception.
func TestRenderTextRendersASelfLinkAsABareURL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Article URL: https://example.com/p",
		renderText(`<p>Article URL: <a href="https://example.com/p">https://example.com/p</a></p>`, true, 400))
}

// The body is rendered as markdown in a webview, so the scheme allow-list is
// the boundary: an anchor keeps its text and loses a target that is not http.
func TestRenderTextDropsANonHTTPHref(t *testing.T) {
	t.Parallel()

	for _, href := range []string{"javascript:alert(1)", "data:text/html,x", "mailto:a@b.c", ""} {
		assert.Equalf(t, "click", renderText(`<a href="`+href+`">click</a>`, true, 400), "href %q", href)
	}
}

// A URL with parentheses ends a markdown link early. CommonMark's angle
// brackets are what that case is for.
func TestRenderTextWrapsAURLThatWouldBreakTheLink(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "[wiki](<https://en.wikipedia.org/wiki/Go_(language)>)",
		renderText(`<a href="https://en.wikipedia.org/wiki/Go_(language)">wiki</a>`, true, 400))
}

// A feed is not held to well-formed HTML, and an unclosed anchor must not
// swallow the rest of the summary.
func TestRenderTextClosesAnUnclosedAnchor(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "[Comments](https://example.com)",
		renderText(`<a href="https://example.com">Comments`, true, 400))
}

func TestEntryLabelsAreCapped(t *testing.T) {
	t.Parallel()

	categories := make([]string, 0, maxLabels+5)
	for range maxLabels + 5 {
		categories = append(categories, "tag")
	}
	item := &gofeed.Item{Title: "t", Link: "https://example.com", Categories: append(categories, "")}

	entry, ok := entryOf(item, "feed")
	require.True(t, ok)
	assert.Len(t, decode(t, entry).Labels, maxLabels)
}
