package tmpl

import (
	"crypto/rand"
	"text/template"
)

// UntrustedFuncs returns untrustedStart and untrustedEnd, which render the
// opening and closing tags that fence outside content (a PR body, a webhook
// payload) in a prompt. The id is part of the tag name rather than an
// attribute so the closing tag carries it too. It is drawn fresh for each
// call, so call this once per render: content written before the render
// cannot know the id, and a closing tag it carries cannot close the fence.
func UntrustedFuncs() template.FuncMap {
	tag := "untrusted-content-" + rand.Text()
	return template.FuncMap{
		"untrustedStart": func() string { return "<" + tag + ">" },
		"untrustedEnd":   func() string { return "</" + tag + ">" },
	}
}
