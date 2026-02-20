package tui

import (
	"testing"

	"github.com/colonyops/hive/internal/core/doctor"
	"github.com/colonyops/hive/internal/tui/components"
	"github.com/stretchr/testify/assert"
)

func TestBuildDoctorDialogContent_StatusMappingAndSummary(t *testing.T) {
	results := []doctor.Result{
		{
			Name: "tools",
			Items: []doctor.CheckItem{
				{Label: "git", Detail: "available", Status: doctor.StatusPass},
				{Label: "tmux", Detail: "missing", Status: doctor.StatusFail},
			},
		},
		{
			Name: "config",
			Items: []doctor.CheckItem{
				{Label: "repo_dirs", Detail: "empty", Status: doctor.StatusWarn},
			},
		},
	}

	sections, footer := buildDoctorDialogContent(results)

	if assert.Len(t, sections, 2) {
		assert.Equal(t, "tools", sections[0].Title)
		if assert.Len(t, sections[0].Items, 2) {
			assert.Equal(t, components.InfoStatusPass, sections[0].Items[0].Status)
			assert.Equal(t, components.InfoStatusFail, sections[0].Items[1].Status)
		}

		assert.Equal(t, "config", sections[1].Title)
		if assert.Len(t, sections[1].Items, 1) {
			assert.Equal(t, components.InfoStatusWarn, sections[1].Items[0].Status)
		}
	}

	assert.Contains(t, footer, "1 passed", "footer should include pass count")
	assert.Contains(t, footer, "1 warnings", "footer should include warning count")
	assert.Contains(t, footer, "1 failed", "footer should include fail count")
}
