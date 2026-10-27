package core

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"

	"github.com/colonyops/hive/internal/domain/drive"
)

// SearchText is the text the search index reads for a document. Markdown is
// indexed as written. HTML loses its tags, comments, and the contents of
// script and style elements, and its entities become characters, so a query
// for "div" does not match every page.
func SearchText(kind drive.Kind, body []byte) string {
	switch kind {
	case drive.KindMarkdown:
		return string(body)
	case drive.KindHtml:
		return htmlText(body)
	case drive.KindDir, drive.KindCanvas, drive.KindFile:
	}
	return ""
}

func htmlText(body []byte) string {
	z := html.NewTokenizer(bytes.NewReader(body))
	var b strings.Builder
	skipDepth := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.TrimSpace(b.String())
		case html.StartTagToken:
			name, _ := z.TagName()
			if isRawTextElement(name) {
				skipDepth++
			}
			b.WriteByte(' ')
		case html.EndTagToken:
			name, _ := z.TagName()
			if isRawTextElement(name) && skipDepth > 0 {
				skipDepth--
			}
			b.WriteByte(' ')
		case html.SelfClosingTagToken:
			b.WriteByte(' ')
		case html.TextToken:
			if skipDepth == 0 {
				b.Write(z.Text())
			}
		case html.CommentToken, html.DoctypeToken:
		}
	}
}

func isRawTextElement(name []byte) bool {
	s := string(name)
	return s == "script" || s == "style"
}
