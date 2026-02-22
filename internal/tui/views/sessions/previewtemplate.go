package sessions

import (
	"bytes"
	"strings"
	"text/template"
)

// PreviewIcons holds nerd font icons for templates.
type PreviewIcons struct {
	Git       string
	GitBranch string
	Github    string
	CheckList string
	Bee       string
	Hive      string
	Claude    string
}

// PreviewGitData holds git status data for templates.
type PreviewGitData struct {
	Branch     string
	Additions  int
	Deletions  int
	HasChanges bool
}

// PreviewPluginData holds plugin status data for templates.
type PreviewPluginData struct {
	Github string // e.g., "open", "draft", "merged"
	Beads  string // e.g., "0/3"
	Claude string // e.g., "65%" (context usage percentage)
}

// PreviewTemplateData holds all data available to preview templates.
type PreviewTemplateData struct {
	// Session data
	Name    string
	ID      string
	ShortID string
	Path    string
	Branch  string // shortcut to GitStatus.Branch

	// Git status
	GitStatus PreviewGitData

	// Plugin data
	Plugin PreviewPluginData

	// Terminal status
	TerminalStatus string

	// Icons
	Icon PreviewIcons
}

// PreviewTemplates holds parsed templates for preview rendering.
type PreviewTemplates struct {
	title  *template.Template
	status *template.Template
}

// DefaultTitleTemplate is the default Go template for the preview title.
// Includes session ID in the title line: "SessionName • #abcd"
const DefaultTitleTemplate = "{{ .Name }} • #{{ .ShortID }}"

// DefaultStatusTemplate is the default Go template for the preview status line.
// Shows git info with icons and plugin statuses.
const DefaultStatusTemplate = "{{ if .Branch }}{{ .Icon.GitBranch }} {{ .Branch }} +{{ .GitStatus.Additions }} -{{ .GitStatus.Deletions }}{{ if .GitStatus.HasChanges }} {{ .Icon.Git }}{{ end }}{{ end }}{{ if .Plugin.Github }} | {{ .Icon.Github }} {{ .Plugin.Github }}{{ end }}{{ if .Plugin.Beads }} | {{ .Icon.CheckList }} {{ .Plugin.Beads }}{{ end }}{{ if .Plugin.Claude }} | {{ .Icon.Claude }} {{ .Plugin.Claude }}{{ end }}"

// ParsePreviewTemplates parses the title and status templates.
// Falls back to defaults if templates are empty or invalid.
func ParsePreviewTemplates(titleTmpl, statusTmpl string) *PreviewTemplates {
	pt := &PreviewTemplates{}

	// Parse title template
	if titleTmpl == "" {
		titleTmpl = DefaultTitleTemplate
	}
	t, err := template.New("title").Parse(titleTmpl)
	if err != nil {
		t, _ = template.New("title").Parse(DefaultTitleTemplate)
	}
	pt.title = t

	// Parse status template
	if statusTmpl == "" {
		statusTmpl = DefaultStatusTemplate
	}
	s, err := template.New("status").Parse(statusTmpl)
	if err != nil {
		s, _ = template.New("status").Parse(DefaultStatusTemplate)
	}
	pt.status = s

	return pt
}

// RenderTitle executes the title template with the given data.
func (pt *PreviewTemplates) RenderTitle(data PreviewTemplateData) string {
	return pt.execute(pt.title, data)
}

// RenderStatus executes the status template with the given data.
func (pt *PreviewTemplates) RenderStatus(data PreviewTemplateData) string {
	return pt.execute(pt.status, data)
}

// execute runs a template and returns the result or an error message.
func (pt *PreviewTemplates) execute(t *template.Template, data PreviewTemplateData) string {
	if t == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "(template error)"
	}
	return strings.TrimSpace(buf.String())
}
