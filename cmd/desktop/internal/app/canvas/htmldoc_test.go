package canvas

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrapDocument(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "meta is the first child of an existing head",
			body: `<!doctype html><html><head><title>t</title></head><body>x</body></html>`,
			want: `<!doctype html><html><head>` + cspMeta + `<title>t</title></head><body>x</body></html>`,
		},
		{
			name: "head with attributes and upper case",
			body: `<HTML lang="en"><HEAD data-x="1"><TITLE>t</TITLE></HEAD></HTML>`,
			want: `<HTML lang="en"><HEAD data-x="1">` + cspMeta + `<TITLE>t</TITLE></HEAD></HTML>`,
		},
		{
			name: "head added inside html",
			body: `<!DOCTYPE html><html><body><p>x</p></body></html>`,
			want: `<!DOCTYPE html><html><head>` + cspMeta + `</head><body><p>x</p></body></html>`,
		},
		{
			name: "existing csp meta stays after ours",
			body: `<html><head><meta http-equiv="Content-Security-Policy" content="default-src *"></head></html>`,
			want: `<html><head>` + cspMeta + `<meta http-equiv="Content-Security-Policy" content="default-src *"></head></html>`,
		},
		{
			name: "bare fragment gains html and head",
			body: `<h1>Report</h1><p>body</p>`,
			want: `<html><head>` + cspMeta + `</head><h1>Report</h1><p>body</p></html>`,
		},
		{
			name: "doctype stays first when html is missing",
			body: "<!doctype html>\n<p>x</p>",
			want: "<!doctype html><html><head>" + cspMeta + "</head>\n<p>x</p></html>",
		},
		{
			name: "head in a comment and a header element do not count",
			body: `<!-- <head> --><header>h</header>`,
			want: `<html><head>` + cspMeta + `</head><!-- <head> --><header>h</header></html>`,
		},
		{
			name: "empty document",
			body: ``,
			want: `<html><head>` + cspMeta + `</head></html>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, WrapDocument([]byte(tc.body)))
		})
	}
}

func TestWrapDocumentMetaCarriesThePolicy(t *testing.T) {
	got := WrapDocument([]byte(`<p>x</p>`))
	assert.Contains(t, got, `http-equiv="Content-Security-Policy" content="`+DocumentCSP+`"`)
}

// Non-whitespace text before <head> makes the parser open <body>, where a CSP
// meta is ignored, so the meta has to precede every such character.
func TestWrapDocumentMetaPrecedesLeadingText(t *testing.T) {
	cases := []struct {
		name string
		body string
		text string
	}{
		{name: "text before head", body: `x<head><title>t</title></head><body></body>`, text: `x<head><title>t</title></head><body></body>`},
		{name: "text between html and head", body: `<html>x<head>`, text: `x<head>`},
		{name: "text before html", body: `x<html>`, text: `x<html>`},
		{name: "text before doctype", body: `x<!doctype html><p>`, text: `x<!doctype html><p>`},
		{name: "byte order mark", body: "\uFEFFx<head></head>", text: `x<head></head>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := WrapDocument([]byte(tc.body))
			assert.True(t, strings.HasPrefix(got, "<html><head>"+cspMeta), "meta is not ahead of all text: %q", got)
			assert.Contains(t, got, tc.text)
		})
	}
}
