package prompts

import (
	"bytes"
	"text/template"
)

func Diagnostics(description, evidence, endpoint string) (string, error) {
	return renderDiagnostics("diagnostics.tmpl", map[string]string{"Description": description, "Evidence": evidence, "Endpoint": endpoint})
}

func DiagnosticsLaunch(path string) (string, error) {
	return renderDiagnostics("diagnostics-launch.tmpl", map[string]string{"Path": path})
}

func renderDiagnostics(name string, data map[string]string) (string, error) {
	tmpl, err := template.ParseFS(templatesFS, "templates/"+name)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	err = tmpl.Execute(&out, data)
	return out.String(), err
}
