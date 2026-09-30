package rss

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
)

// maxBodyText caps, in runes, the text carried into an item's body. A
// full-content feed ships whole posts, and an inbox row's detail pane is not a
// reader.
const maxBodyText = 4000

// maxTitleText caps a title. It is a row's one line, and a feed that puts its
// summary in the title element should not take the row with it.
const maxTitleText = 300

// maxLabels caps the tags taken from an entry's categories. A handful
// describes a post; a hundred is a keyword-stuffed feed filling the row.
const maxLabels = 20

// Entry is one feed entry reduced to what the canonical item contract needs.
// The cache holds these rather than a parsed document, so what a 304 re-emits
// is exactly what the last 200 emitted.
type Entry struct {
	// Key is the entry's identity across fetches.
	Key string
	// Payload is the canonical item JSON, built at parse time because a 304
	// re-emits it unchanged.
	Payload json.RawMessage
	// at orders the window, newest first. Zero for an entry the feed dated
	// neither way.
	at time.Time
}

// payload is the canonical item contract an entry fills
// (docs/decisions/2026-07-24-canonical-item-contract.md).
//
// No `state`: a feed entry has no lifecycle to report, so every entry stays
// active until the flow's retention ages it out. Anything that read a state
// here would be inventing one.
type payload struct {
	Kind string `json:"kind"`
	// Repo is the canonical container field, which for a feed is the feed's
	// own title — what the row renders above the entry.
	Repo      string   `json:"repo,omitempty"`
	Title     string   `json:"title"`
	URL       string   `json:"url,omitempty"`
	Body      string   `json:"body,omitempty"`
	Author    string   `json:"author,omitempty"`
	Labels    []string `json:"labels,omitempty"`
	Published string   `json:"published,omitempty"`
	Updated   string   `json:"updated,omitempty"`
}

// entriesOf reduces a parsed feed to its entries, newest first, with at most
// one entry per key.
//
// A publisher that reuses a GUID across two entries is a feed bug, but not one
// worth failing the whole window over: the newer entry wins and the other is
// dropped, where an error would take the other forty entries down with it.
func entriesOf(feed *gofeed.Feed) []Entry {
	if feed == nil {
		return nil
	}
	feedTitle := strings.TrimSpace(feed.Title)

	entries := make([]Entry, 0, len(feed.Items))
	for _, item := range feed.Items {
		if item == nil {
			continue
		}
		entry, ok := entryOf(item, feedTitle)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}

	// Stable, so entries a feed dated neither way keep the order it published
	// them in — they sort last, where "newest first" has nothing to say.
	sort.SliceStable(entries, func(i, j int) bool {
		left, right := entries[i].at, entries[j].at
		if left.IsZero() || right.IsZero() {
			return !left.IsZero() && right.IsZero()
		}
		return left.After(right)
	})

	seen := make(map[string]struct{}, len(entries))
	deduped := entries[:0]
	for _, entry := range entries {
		if _, dup := seen[entry.Key]; dup {
			continue
		}
		seen[entry.Key] = struct{}{}
		deduped = append(deduped, entry)
	}
	return deduped
}

// entryOf builds one entry. It reports false for an entry with nothing to
// identify or show, which is a feed's own malformed row rather than a failure
// of the fetch.
func entryOf(item *gofeed.Item, feedTitle string) (Entry, bool) {
	title := renderText(item.Title, false, maxTitleText)
	link := strings.TrimSpace(item.Link)
	key := entryKey(item, title)
	if key == "" || (title == "" && link == "") {
		return Entry{}, false
	}
	if title == "" {
		title = link
	}

	body, err := json.Marshal(payload{
		Kind:      ItemKind,
		Repo:      feedTitle,
		Title:     title,
		URL:       link,
		Body:      entryBody(item),
		Author:    entryAuthor(item),
		Labels:    entryLabels(item),
		Published: formatTime(item.PublishedParsed),
		Updated:   formatTime(item.UpdatedParsed),
	})
	if err != nil {
		return Entry{}, false
	}
	return Entry{Key: key, Payload: body, at: entryTime(item)}, true
}

// entryKey is the entry's identity across fetches. The GUID (an Atom id) is
// what a feed publishes for exactly this, the link is the near-universal
// stand-in, and the digest is the last resort — it makes an edited title a new
// item, which is wrong but bounded, where keying on nothing would make every
// fetch a new item.
func entryKey(item *gofeed.Item, title string) string {
	if guid := strings.TrimSpace(item.GUID); guid != "" {
		return guid
	}
	if link := strings.TrimSpace(item.Link); link != "" {
		return link
	}
	if title == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(title + "\x00" + formatTime(item.PublishedParsed)))
	return hex.EncodeToString(sum[:8])
}

// entryBody is the detail pane's text. The summary is preferred over the
// content because a full-content feed ships the whole post, and the pane is
// there to say what the entry is, not to be a reader.
func entryBody(item *gofeed.Item) string {
	if summary := renderText(item.Description, true, maxBodyText); summary != "" {
		return summary
	}
	return renderText(item.Content, true, maxBodyText)
}

func entryAuthor(item *gofeed.Item) string {
	for _, author := range item.Authors {
		if author == nil {
			continue
		}
		if name := strings.TrimSpace(author.Name); name != "" {
			return name
		}
	}
	return ""
}

func entryLabels(item *gofeed.Item) []string {
	labels := make([]string, 0, min(len(item.Categories), maxLabels))
	for _, category := range item.Categories {
		category = strings.TrimSpace(category)
		if category == "" {
			continue
		}
		labels = append(labels, category)
		if len(labels) == maxLabels {
			break
		}
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}

// entryTime orders the window. Published is preferred over updated so a post
// edited today does not jump ahead of one written today.
func entryTime(item *gofeed.Item) time.Time {
	if item.PublishedParsed != nil {
		return *item.PublishedParsed
	}
	if item.UpdatedParsed != nil {
		return *item.UpdatedParsed
	}
	return time.Time{}
}

func formatTime(at *time.Time) string {
	if at == nil {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}

// renderText reduces a feed's HTML to the text a payload field carries. With
// links, an anchor becomes a markdown link, which is what the detail pane
// renders; without, only its text survives, because a title is not markdown.
//
// A tokenizer rather than a tag stripper, for two reasons a strip cannot
// cover: an anchor's href is the only thing some feeds put in a summary (HN's
// is a bare link to the comments), and a strip runs "<p>a</p><p>b</p>"
// together into "ab" where a block boundary is a word boundary.
func renderText(raw string, links bool, max int) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	var out strings.Builder
	var anchor strings.Builder
	href := ""
	inAnchor := false

	write := func(text string) {
		if inAnchor {
			anchor.WriteString(text)
			return
		}
		out.WriteString(text)
	}

	z := html.NewTokenizer(strings.NewReader(raw))
	for {
		switch z.Next() {
		case html.ErrorToken:
			if inAnchor {
				out.WriteString(markdownLink(anchor.String(), href))
			}
			return collapse(out.String(), max)

		case html.TextToken:
			write(string(z.Text()))

		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := string(name)
			if links && tag == "a" && !inAnchor {
				href, inAnchor = linkHref(z, hasAttr), true
				anchor.Reset()
				continue
			}
			if blockTags[tag] {
				write(" ")
			}

		case html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if tag == "a" && inAnchor {
				inAnchor = false
				out.WriteString(markdownLink(anchor.String(), href))
				continue
			}
			if blockTags[tag] {
				write(" ")
			}

		case html.CommentToken, html.DoctypeToken:
			// Neither carries text a summary should show.
		}
	}
}

// blockTags are the elements whose boundary is a word boundary. The set is
// deliberately short: it only has to stop text running together, not model
// HTML layout.
var blockTags = map[string]bool{
	"br": true, "p": true, "div": true, "li": true, "tr": true, "td": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"blockquote": true, "pre": true, "section": true, "article": true,
}

// linkHref reads an anchor's href, keeping only the schemes safe to put in
// markdown a webview renders. Anything else (javascript:, data:) returns
// empty, which keeps the anchor's text and drops the target.
func linkHref(z *html.Tokenizer, hasAttr bool) string {
	for hasAttr {
		var key, val []byte
		key, val, hasAttr = z.TagAttr()
		if string(key) != "href" {
			continue
		}
		href := strings.TrimSpace(string(val))
		lower := strings.ToLower(href)
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			return href
		}
		return ""
	}
	return ""
}

// markdownLink renders one anchor. A link whose text is its own URL renders as
// the bare URL, because "[https://x](https://x)" is noise; a URL carrying
// parentheses or spaces takes the angle-bracket form CommonMark provides for
// exactly that.
func markdownLink(text, href string) string {
	text = strings.TrimSpace(text)
	switch {
	case href == "":
		return text
	case text == "" || text == href:
		return href
	}
	if strings.ContainsAny(href, "() ") {
		href = "<" + href + ">"
	}
	return "[" + strings.ReplaceAll(text, "]", "\\]") + "](" + href + ")"
}

func collapse(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	return strings.TrimSpace(string([]rune(text)[:max])) + "\u2026"
}
