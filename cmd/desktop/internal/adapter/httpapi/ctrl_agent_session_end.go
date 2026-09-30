package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/server"
)

type agentSessionEndResponse struct {
	Session int64 `json:"session"`
	EndsAt  int64 `json:"endsAt"`
}

// AgentSessionEnd ends the calling chat's own session. The bearer is the token
// the launch handed that process, not the terminal token: a chat holds a
// capability over itself and nothing else, which is why this route sits beside
// the liveness probe rather than under the token-guarded terminal prefix.
func (ctrl *Controller) AgentSessionEnd(w http.ResponseWriter, r *http.Request) error {
	ending, err := ctrl.core.AgentWorkspaces.EndOwnSession(r.Context(), bearerToken(r))
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusAccepted, agentSessionEndResponse{
		Session: ending.Session.ID, EndsAt: ending.EndsAt.UnixMilli(),
	})
}
