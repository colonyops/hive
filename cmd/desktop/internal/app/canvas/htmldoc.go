package canvas

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// DocumentCSP is the policy an html document runs under: https and data
// sources only, so a page cannot reach the loopback MCP server, submit a form,
// or frame anything. Inline and https scripts stay allowed on purpose: agent
// pages are meant to run, and the sandboxed frame, not this policy, is the
// boundary
// (ADR hive-drive-is-one-document-tree-under-the-context-root-and-a-canvas-is-one-document-kind-in-it).
const DocumentCSP = "default-src 'none'; script-src 'unsafe-inline' https:; style-src 'unsafe-inline' https:; img-src data: https:; font-src data: https:; media-src data: https:; connect-src https:; frame-src 'none'; form-action 'none'; base-uri 'none'"

const htmlWhitespace = "\t\n\f\r "

// A srcdoc document inherits its parent's base URL, so without this base a
// fragment or relative link resolves against the app's own URL and the frame
// loads Hive inside itself. Against about:srcdoc a fragment scrolls the page
// and a relative path does not resolve, so the link does nothing. The base
// precedes the CSP meta because base-uri 'none' would refuse it after, and
// still refuses one the document declares.
//
// The meta needs no attribute escaping: DocumentCSP holds no double quote.
const injectedHead = `<base href="about:srcdoc">` +
	`<meta http-equiv="Content-Security-Policy" content="` + DocumentCSP + `">`

// WrapDocument returns body with injectedHead (a base URL and a CSP meta) as
// the first elements of <head>, adding <head>, and <html> when the document
// has neither. A policy the document declares itself stays after ours; the
// browser enforces both, so a document can only tighten the policy, never
// loosen it.
func WrapDocument(body []byte) string {
	body = bytes.TrimPrefix(body, []byte("\uFEFF"))
	headEnd, htmlEnd, doctypeEnd := documentAnchors(body)
	var out strings.Builder
	out.Grow(len(body) + len(injectedHead) + 32)
	switch {
	case headEnd >= 0:
		out.Write(body[:headEnd])
		out.WriteString(injectedHead)
		out.Write(body[headEnd:])
	case htmlEnd >= 0:
		out.Write(body[:htmlEnd])
		out.WriteString("<head>" + injectedHead + "</head>")
		out.Write(body[htmlEnd:])
	default:
		prefix := max(doctypeEnd, 0)
		out.Write(body[:prefix])
		out.WriteString("<html><head>" + injectedHead + "</head>")
		out.Write(body[prefix:])
		out.WriteString("</html>")
	}
	return out.String()
}

// documentAnchors finds the byte offsets just past the document's <head>,
// <html>, and doctype tokens, or -1 for each it lacks. A tokenizer rather than
// a substring search, so a "<head>" inside a comment or a "<header>" never
// matches. Only the prologue is read: any other tag, or any text that is not
// whitespace, ends it. The parser opens <body> at the first such text, and a
// CSP meta after that point is ignored, so a later <head> does not count and a
// doctype after it is not kept in front.
func documentAnchors(body []byte) (headEnd, htmlEnd, doctypeEnd int) {
	headEnd, htmlEnd, doctypeEnd = -1, -1, -1
	z := html.NewTokenizer(bytes.NewReader(body))
	offset := 0
	for {
		tt := z.Next()
		offset += len(z.Raw())
		switch tt {
		case html.ErrorToken, html.EndTagToken:
			return headEnd, htmlEnd, doctypeEnd
		case html.DoctypeToken:
			doctypeEnd = offset
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "head":
				return offset, htmlEnd, doctypeEnd
			case "html":
				htmlEnd = offset
			default:
				return headEnd, htmlEnd, doctypeEnd
			}
		case html.TextToken:
			if strings.TrimLeft(string(z.Raw()), htmlWhitespace) != "" {
				return headEnd, htmlEnd, doctypeEnd
			}
		case html.CommentToken:
		}
	}
}
