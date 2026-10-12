package canvas

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// DocumentCSP is the policy an html document runs under: https and data
// sources only, so a page cannot reach the loopback MCP server, submit a form,
// or frame anything.
const DocumentCSP = "default-src 'none'; script-src 'unsafe-inline' https:; style-src 'unsafe-inline' https:; img-src data: https:; font-src data: https:; media-src data: https:; connect-src https:; frame-src 'none'; form-action 'none'; base-uri 'none'"

// cspMeta needs no attribute escaping: DocumentCSP holds no double quote.
const cspMeta = `<meta http-equiv="Content-Security-Policy" content="` + DocumentCSP + `">`

// WrapDocument returns body with a CSP meta as the first element of <head>,
// adding <head>, and <html> when the document has neither. A policy the
// document declares itself stays after ours; the browser enforces both, so a
// document can only tighten the policy, never loosen it.
func WrapDocument(body []byte) string {
	headEnd, htmlEnd, doctypeEnd := documentAnchors(body)
	var out strings.Builder
	out.Grow(len(body) + len(cspMeta) + 32)
	switch {
	case headEnd >= 0:
		out.Write(body[:headEnd])
		out.WriteString(cspMeta)
		out.Write(body[headEnd:])
	case htmlEnd >= 0:
		out.Write(body[:htmlEnd])
		out.WriteString("<head>" + cspMeta + "</head>")
		out.Write(body[htmlEnd:])
	default:
		prefix := max(doctypeEnd, 0)
		out.Write(body[:prefix])
		out.WriteString("<html><head>" + cspMeta + "</head>")
		out.Write(body[prefix:])
		out.WriteString("</html>")
	}
	return out.String()
}

// documentAnchors finds the byte offsets just past the document's <head>,
// <html>, and doctype tokens, or -1 for each it lacks. A tokenizer rather than
// a substring search, so a "<head>" inside a comment or a "<header>" never
// matches. Only the prologue is read: any other tag first means the document
// has no explicit head.
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
		case html.TextToken, html.CommentToken:
		}
	}
}
