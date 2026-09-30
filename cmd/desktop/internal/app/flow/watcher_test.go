package flow

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsFlowFile(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"triage.yaml", true},
		{"triage.yml", true},
		{"triage.ui.yaml", true}, // layout edits still trigger a reload
		{"triage.ui.yml", true},
		{"triage.sidebar.yaml", false}, // sidebar layout is frontend-owned UI state
		{"triage.sidebar.yml", false},
		{"triage.sidebar.yaml.tmp", false}, // atomic-write temp file
		{"triage.yaml.tmp", false},
		{"notes.txt", false},
		{"triage", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isFlowFile(filepath.Join("/flows", tc.name)))
		})
	}
}
