package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditableActionRoundTripsEverySchemaBranchWithoutLoss(t *testing.T) {
	for _, editable := range []EditableAction{
		{ID: "launch", Label: "Launch", Type: "launch-session", ShowInDetail: true, AppliesTo: []string{"pr"}, Launch: &EditableLaunchConfig{PromptTemplate: "prompt", RepoTemplate: "repo", Agent: "agent"}},
		{ID: "shell", Label: "Shell", Type: "shell", ShowInDetail: true, Shell: &EditableShellConfig{CommandTemplate: "run", Cwd: "/work", Timeout: "30s", Env: map[string]string{"B": "2", "A": "1"}}},
		{ID: "event", Label: "Event", Type: "publish-message", Message: &EditableMessageConfig{MessageTemplate: "message", Topic: "topic"}},
	} {
		t.Run(editable.Type, func(t *testing.T) {
			action, err := actionFromEditable(editable)
			require.NoError(t, err)
			assert.Equal(t, editable, editableFromAction(action))
		})
	}
}
