package canvas

import (
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

// The html block kind: the app decides what a block can reach, and the agent
// decides what it looks like
// (ADR a-canvas-html-block-is-restricted-by-what-it-can-reach-not-by-how-it-looks).
// Two allowlists are the whole contract — the elements that survive and the
// attributes that survive. Inside them nothing is policed: any class, any
// value. A block cannot paint outside its own pane, so the worst a bad one
// does is make itself ugly.

// htmlElements is the structure an agent may write. Deny-by-default, and
// every name left out is left out for something it can reach: script, style,
// iframe, object, embed and form execute, load, or send, and math, template
// and the svg integration points below are where a second parse disagrees
// with the first. Layout and taste are not reasons — those belong to the
// agent.
var htmlElements = []string{
	"a",
	"abbr",
	"article",
	"aside",
	"b",
	"blockquote",
	"br",
	"caption",
	"code",
	"col",
	"colgroup",
	"dd",
	"details",
	"div",
	"dl",
	"dt",
	"em",
	"figcaption",
	"figure",
	"footer",
	"h1",
	"h2",
	"h3",
	"h4",
	"h5",
	"h6",
	"header",
	"hr",
	"i",
	"img",
	"kbd",
	"li",
	"mark",
	"ol",
	"p",
	"pre",
	"q",
	"s",
	"samp",
	"section",
	"small",
	"span",
	"strong",
	"sub",
	"summary",
	"sup",
	"table",
	"tbody",
	"td",
	"tfoot",
	"th",
	"thead",
	"time",
	"tr",
	"u",
	"ul",
	"var",
}

// svgElements is the drawing half: enough to place boxes, join them, and
// label both. desc, title and foreignObject are the integration points where
// a browser parses HTML again inside foreign content, so they stay out with
// their content; script and style execute; use and image fetch. defs and
// marker are left out for a duller reason — they need ids, and an id in this
// page can clobber a global the app's own code reads.
var svgElements = []string{
	"circle",
	"ellipse",
	"g",
	"line",
	"path",
	"polygon",
	"polyline",
	"rect",
	"svg",
	"text",
	"tspan",
}

// htmlAttrs is every attribute a block may carry, on any element that accepts
// it, with any value. Deny-by-default again, and again for reach rather than
// for taste: no on* handler, no srcdoc, and no id, because an id in this page
// can clobber a global the app's own code reads. style is here and carries
// whatever the agent writes — it is the only override that beats a rule in
// canvas-html.css, so without it the defaults below would be walls rather
// than defaults.
var htmlAttrs = []string{
	"alt",
	"class",
	"colspan",
	"cx",
	"cy",
	"d",
	"dir",
	"dominant-baseline",
	"dx",
	"dy",
	"fill",
	"fill-opacity",
	"fill-rule",
	"font-family",
	"font-size",
	"font-style",
	"font-weight",
	"height",
	"lang",
	"letter-spacing",
	"opacity",
	"open",
	"paint-order",
	"points",
	"preserveaspectratio",
	"r",
	"rowspan",
	"rx",
	"ry",
	"scope",
	"stroke",
	"stroke-dasharray",
	"stroke-linecap",
	"stroke-linejoin",
	"stroke-opacity",
	"stroke-width",
	"style",
	"text-anchor",
	"title",
	"transform",
	"vector-effect",
	"viewbox",
	"width",
	"x",
	"x1",
	"x2",
	"y",
	"y1",
	"y2",
}

// htmlClasses is what the stylesheet defines and the doc page teaches, so an
// agent that wants the app's own look has names to reach for and two canvases
// written months apart still match. It is a vocabulary, not a gate: a class
// outside it is the agent's business and reaches the DOM untouched.
var htmlClasses = []string{
	// Layout.
	"hv-stack",
	"hv-row",
	"hv-grid",
	"hv-cols-2",
	"hv-cols-3",
	"hv-cols-4",
	// Containers.
	"hv-card",
	"hv-panel",
	"hv-callout",
	// Data.
	"hv-stat",
	"hv-stat-value",
	"hv-stat-label",
	"hv-kv",
	// Emphasis.
	"hv-badge",
	"hv-muted",
	"hv-mono",
	// Diagram roles, inside an svg.
	"hv-node",
	"hv-edge",
	"hv-arrow",
	"hv-dashed",
	"hv-label",
	// Tones, for hv-callout, hv-badge and the diagram roles.
	"hv-info",
	"hv-success",
	"hv-warn",
	"hv-error",
	"hv-accent",
}

// HTMLElements and HTMLClasses report the element allowlist and the styled
// vocabulary, for the write-time error messages and the documentation test.
func HTMLElements() []string {
	return append(append([]string(nil), htmlElements...), svgElements...)
}
func HTMLClasses() []string { return append([]string(nil), htmlClasses...) }

var htmlPolicy = sync.OnceValue(func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements(htmlElements...)
	p.AllowElements(svgElements...)
	// A <g> or a <text> carries no attribute of its own when a class does the
	// work, and bluemonday unwraps an attribute-less element unless it is
	// told otherwise.
	p.AllowNoAttrs().OnElements(svgElements...)
	p.AllowAttrs(htmlAttrs...).Globally()

	// The two attributes that carry a URL, and the only values the policy
	// rules on. href takes the link block's schemes, so a block never renders
	// a link the pane's click handler then refuses to open. Relative URLs
	// stay out: there is nothing in the webview for one to resolve against.
	// An img reaches whatever host it names, which the user has agreed to.
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("src").OnElements("img")
	p.AllowURLSchemes("http", "https", "mailto")
	p.AllowDataURIImages()
	p.RequireParseableURLs(true)
	p.AllowRelativeURLs(false)

	// v-html feeds this through innerHTML, which re-parses it. These are the
	// elements a second parse reads differently from the tokenizer this
	// policy runs on — math and template outright, and the three svg
	// integration points, where markup inside foreign content becomes HTML
	// again. Drop what they carry rather than letting it re-enter as markup.
	p.SkipElementsContent("math", "template", "form", "select", "textarea", "foreignobject", "desc", "title")
	return p
})

// svgCasing restores the one camelCase name in the allowlist. SVG attribute
// names are case-sensitive and the HTML tokenizer folds them, so the policy
// emits viewbox; a browser is meant to fold it back when it re-parses the
// markup as foreign content, but jsdom does not, and the stylesheet selects
// on svg[viewBox] to decide whether it may scale a drawing. Text tokens
// escape their quotes, so the sequence this matches only ever occurs in an
// attribute position.
var svgCasing = strings.NewReplacer(` viewbox="`, ` viewBox="`)

// SanitizeHTML renders an html block's source as markup that is safe to put
// in the app's own webview. The stored source is never touched — an agent
// reads back what it wrote, and the sanitized form is what leaves the app.
func SanitizeHTML(src string) string {
	return svgCasing.Replace(htmlPolicy().Sanitize(src))
}

// RejectedHTML names the first element, attribute or link in src that
// SanitizeHTML would silently drop, so a write fails with the name instead of
// rendering a canvas the agent believes is intact. An empty string means the
// source reaches the pane whole.
func RejectedHTML(src string) string {
	z := html.NewTokenizer(strings.NewReader(src))
	for {
		token := z.Next()
		if token == html.ErrorToken {
			return ""
		}
		if token != html.StartTagToken && token != html.SelfClosingTagToken {
			continue
		}
		name, hasAttr := z.TagName()
		tag := string(name)
		if !slices.Contains(htmlElements, tag) && !slices.Contains(svgElements, tag) {
			return "the <" + tag + "> element"
		}
		for hasAttr {
			var rawKey, rawVal []byte
			rawKey, rawVal, hasAttr = z.TagAttr()
			attr, value := string(rawKey), string(rawVal)
			switch {
			case attr == "href" && tag == "a":
				if rejectedURL(tag, attr, value) {
					return "the link " + strconv.Quote(value)
				}
			case attr == "src" && tag == "img":
				if rejectedURL(tag, attr, value) {
					return "the image source " + strconv.Quote(value)
				}
			case !slices.Contains(htmlAttrs, attr):
				return "the " + attr + " attribute on <" + tag + ">"
			}
		}
	}
}

// rejectedURL asks the policy whether a URL survives, instead of restating
// its scheme rule here. The policy drops the whole element for a URL it
// refuses, which reads to the agent as a link that vanished, and a second
// copy of the rule is how a write starts accepting what the render drops.
func rejectedURL(tag, attr, raw string) bool {
	probe := "<" + tag + " " + attr + `="` + html.EscapeString(raw) + `">`
	return !strings.Contains(SanitizeHTML(probe), attr+`="`)
}
