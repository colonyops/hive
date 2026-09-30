package schedule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPromptData() PromptData {
	now := time.Date(2026, time.September, 4, 9, 0, 0, 0, time.UTC)
	data := PromptData{
		Now:          now,
		ScheduledFor: now,
		Reason:       string(ReasonDue),
		Missed:       2,
	}
	data.Schedule.ID = "weekly-summary"
	data.Schedule.Name = "Weekly product summary"
	data.Schedule.Cron = "0 9 * * 5"
	data.Workspace.Dir = "product"
	data.Workspace.Name = "Product"
	return data
}

func TestRenderPrompt(t *testing.T) {
	t.Parallel()

	lastRun := time.Date(2026, time.August, 28, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		tmpl    string
		lastRun *time.Time
		want    string
		wantErr bool
	}{
		{
			name: "plain text passes through",
			tmpl: "Summarize the week.",
			want: "Summarize the week.",
		},
		{
			name: "fields",
			tmpl: "{{ .Schedule.Name }} in {{ .Workspace.Name }} ({{ .Workspace.Dir }}), {{ .Reason }}, missed {{ .Missed }}",
			want: "Weekly product summary in Product (product), due, missed 2",
		},
		{
			name: "date over a time",
			tmpl: `{{ date "2006-01-02" .Now }}`,
			want: "2026-09-04",
		},
		{
			name:    "date over a last run",
			tmpl:    `since {{ date "2006-01-02" .LastRun }}`,
			lastRun: &lastRun,
			want:    "since 2026-08-28",
		},
		{
			name: "date over a nil last run renders empty",
			tmpl: `since [{{ date "2006-01-02" .LastRun }}]`,
			want: "since []",
		},
		{
			name:    "a template that does not parse",
			tmpl:    "{{ .Now",
			wantErr: true,
		},
		{
			name:    "an unknown field",
			tmpl:    "{{ .NoSuchField }}",
			wantErr: true,
		},
		{
			name:    "an unknown function",
			tmpl:    "{{ nosuchfunc .Now }}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data := testPromptData()
			data.LastRun = tt.lastRun

			got, err := RenderPrompt(tt.tmpl, data)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestValidatePromptRendersBothRuns is the point of the sample data: a
// template reading .LastRun through the date func has to validate at edit
// time, and one that calls a method on it has to fail there too, not on the
// first Friday morning it finally runs with no previous run to point at.
func TestValidatePromptRendersBothRuns(t *testing.T) {
	t.Parallel()

	require.NotNil(t, SamplePromptData().LastRun)

	tests := []struct {
		name    string
		tmpl    string
		wantErr string
	}{
		{name: "plain", tmpl: "hello"},
		{name: "date over last run", tmpl: `{{ date "2006-01-02" .LastRun }}`},
		{name: "guarded method on last run", tmpl: `{{ if .LastRun }}{{ .LastRun.Format "2006" }}{{ end }}`},
		{name: "every field", tmpl: "{{ .Schedule.ID }}{{ .Schedule.Name }}{{ .Schedule.Cron }}{{ .Workspace.Dir }}{{ .Workspace.Name }}{{ .ScheduledFor }}{{ .Reason }}{{ .Missed }}"},
		{name: "unparseable", tmpl: "{{", wantErr: "prompt template"},
		{name: "unguarded method on last run", tmpl: `{{ .LastRun.Format "2006" }}`, wantErr: "on the first run, with .LastRun unset"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidatePrompt(tt.tmpl)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPreviewPromptRendersTheFirstRunToo(t *testing.T) {
	t.Parallel()

	lastRun := time.Date(2026, time.August, 28, 9, 0, 0, 0, time.UTC)
	data := testPromptData()
	data.LastRun = &lastRun

	preview, err := PreviewPrompt(`Since {{ if .LastRun }}{{ date "2006-01-02" .LastRun }}{{ else }}the start{{ end }}.`, data)
	require.NoError(t, err)
	assert.Equal(t, "Since 2026-08-28.", preview.Prompt)
	assert.Equal(t, "Since the start.", preview.FirstRunPrompt)
}
