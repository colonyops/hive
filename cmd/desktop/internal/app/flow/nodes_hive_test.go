package flow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishMessageConfig_Validate(t *testing.T) {
	t.Parallel()
	require.NoError(t, (&PublishMessageConfig{Topic: "deploys.prod", Message: "{{ .Payload.title }}"}).Validate(nil))

	for name, cfg := range map[string]PublishMessageConfig{
		"no topic":        {Message: "m"},
		"wildcard topic":  {Topic: "agent.*", Message: "m"},
		"templated topic": {Topic: "agent.{{ .Key }}", Message: "m"},
		"padded topic":    {Topic: " agent.x", Message: "m"},
		"no message":      {Topic: "agent.x"},
		"broken template": {Topic: "agent.x", Message: "{{ .Payload"},
	} {
		assert.Errorf(t, cfg.Validate(nil), "%s", name)
	}
}

func TestCreateTaskConfig_Validate(t *testing.T) {
	t.Parallel()
	require.NoError(t, (&CreateTaskConfig{Title: "Fix {{ .Payload.title }}"}).Validate(nil))
	require.NoError(t, (&CreateTaskConfig{
		Title: "t", Description: "{{ .Payload.url }}", Repo: "{{ .Payload.repo }}", Parent: "hc-abc123",
		Tags: []string{"triage", "from-flow"},
	}).Validate(nil))

	for name, cfg := range map[string]CreateTaskConfig{
		"no title":            {},
		"broken title":        {Title: "{{ .Payload"},
		"broken description":  {Title: "t", Description: "{{ end }}"},
		"broken repo":         {Title: "t", Repo: "{{"},
		"templated parent":    {Title: "t", Parent: "{{ .Key }}"},
		"parent with a space": {Title: "t", Parent: "hc 1"},
		"templated tag":       {Title: "t", Tags: []string{"{{ .Key }}"}},
		"tag with a comma":    {Title: "t", Tags: []string{"a,b"}},
	} {
		assert.Errorf(t, cfg.Validate(nil), "%s", name)
	}
}
