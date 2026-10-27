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
			want: `<!doctype html><html><head>` + injectedHead + `<title>t</title></head><body>x</body></html>`,
		},
		{
			name: "head with attributes and upper case",
			body: `<HTML lang="en"><HEAD data-x="1"><TITLE>t</TITLE></HEAD></HTML>`,
			want: `<HTML lang="en"><HEAD data-x="1">` + injectedHead + `<TITLE>t</TITLE></HEAD></HTML>`,
		},
		{
			name: "head added inside html",
			body: `<!DOCTYPE html><html><body><p>x</p></body></html>`,
			want: `<!DOCTYPE html><html><head>` + injectedHead + `</head><body><p>x</p></body></html>`,
		},
		{
			name: "existing csp meta stays after ours",
			body: `<html><head><meta http-equiv="Content-Security-Policy" content="default-src *"></head></html>`,
			want: `<html><head>` + injectedHead + `<meta http-equiv="Content-Security-Policy" content="default-src *"></head></html>`,
		},
		{
			name: "bare fragment gains html and head",
			body: `<h1>Report</h1><p>body</p>`,
			want: `<html><head>` + injectedHead + `</head><h1>Report</h1><p>body</p></html>`,
		},
		{
			name: "doctype stays first when html is missing",
			body: "<!doctype html>\n<p>x</p>",
			want: "<!doctype html><html><head>" + injectedHead + "</head>\n<p>x</p></html>",
		},
		{
			name: "head in a comment and a header element do not count",
			body: `<!-- <head> --><header>h</header>`,
			want: `<html><head>` + injectedHead + `</head><!-- <head> --><header>h</header></html>`,
		},
		{
			name: "empty document",
			body: ``,
			want: `<html><head>` + injectedHead + `</head></html>`,
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

// base-uri 'none' refuses a base element that follows the policy, so ours has
// to come first for the frame to resolve links against about:srcdoc.
func TestWrapDocumentBasePrecedesThePolicy(t *testing.T) {
	got := WrapDocument([]byte(`<head><base href="https://x.example/"></head>`))
	base := strings.Index(got, `<base href="about:srcdoc">`)
	meta := strings.Index(got, `http-equiv="Content-Security-Policy"`)
	theirs := strings.Index(got, `<base href="https://x.example/">`)
	assert.True(t, base >= 0 && base < meta && meta < theirs, "unexpected order: %q", got)
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
			assert.True(t, strings.HasPrefix(got, "<html><head>"+injectedHead), "meta is not ahead of all text: %q", got)
			assert.Contains(t, got, tc.text)
		})
	}
}
