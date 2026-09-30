package schedule

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpecDisplayName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Weekly summary", Spec{ID: "weekly", Name: "Weekly summary"}.DisplayName())
	assert.Equal(t, "weekly", Spec{ID: "weekly"}.DisplayName())
}

func TestSpecValidate(t *testing.T) {
	t.Parallel()

	valid := Spec{ID: "weekly-summary", Cron: "0 9 * * 5", Prompt: "Summarize the week."}

	tests := []struct {
		name    string
		mutate  func(*Spec)
		wantErr bool
	}{
		{name: "valid", mutate: func(*Spec) {}},
		{name: "workspace is not validated here", mutate: func(s *Spec) { s.Workspace = "" }},
		{name: "digits only id", mutate: func(s *Spec) { s.ID = "0" }},
		{name: "on_missed run", mutate: func(s *Spec) { s.OnMissed = OnMissedRun }},
		{name: "on_missed skip", mutate: func(s *Spec) { s.OnMissed = OnMissedSkip }},
		{name: "disabled", mutate: func(s *Spec) { s.Disabled = true }},
		{name: "descriptor cron", mutate: func(s *Spec) { s.Cron = "@daily" }},

		{name: "no id", mutate: func(s *Spec) { s.ID = "" }, wantErr: true},
		{name: "id with a space", mutate: func(s *Spec) { s.ID = "weekly summary" }, wantErr: true},
		{name: "no cron", mutate: func(s *Spec) { s.Cron = "" }, wantErr: true},
		{name: "bad cron", mutate: func(s *Spec) { s.Cron = "0 9 * *" }, wantErr: true},
		{name: "no prompt", mutate: func(s *Spec) { s.Prompt = "" }, wantErr: true},
		{name: "blank prompt", mutate: func(s *Spec) { s.Prompt = "  \n\t" }, wantErr: true},
		{name: "prompt that does not parse", mutate: func(s *Spec) { s.Prompt = "{{ .Now" }, wantErr: true},
		{name: "unknown on_missed", mutate: func(s *Spec) { s.OnMissed = OnMissed("later") }, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec := valid
			tt.mutate(&spec)

			err := spec.Validate()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
