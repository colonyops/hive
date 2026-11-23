//go:build remote

package remote

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/hive/status"
)

// A review from the Changes view goes to the agent the way Desktop sends it:
// the server's daemon names the agent's pane, and the server's tmux, reached
// over ssh, takes the text.
func TestServerAgents_ListThePanesAndTakeAReview(t *testing.T) {
	created := serverA.CreateSession(t, "review-send")
	api := serverA.API(t)

	var agent status.AgentPane
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		panes, err := api.AgentPanes(t.Context(), []string{created.ID})
		if !assert.NoError(c, err) || !assert.NotEmpty(c, panes[created.ID]) {
			return
		}
		agent = panes[created.ID][0]
		assert.Equal(c, "claude", agent.Tool)
	}, 20*time.Second, 500*time.Millisecond, "the server finds the fake claude agent")
	assert.NotEmpty(t, agent.PaneID)
	assert.NotEmpty(t, agent.Status)

	m := serverTerminals(t, serverA)
	require.NoError(t, m.SendText(t.Context(), agent.PaneID, []byte("review-marker-$((6*7))"), 50*time.Millisecond))
	awaitCapture(t, m, agent.PaneID, "review-marker-")

	missing, err := api.AgentPanes(t.Context(), []string{"no-such-session"})
	require.NoError(t, err)
	assert.Empty(t, missing)
}
