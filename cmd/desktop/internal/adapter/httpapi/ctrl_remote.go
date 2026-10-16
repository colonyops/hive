package httpapi

import (
	"net/http"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"

	"github.com/hay-kot/httpkit/server"
)

const ConnectionFileEnv = "HIVE_DESKTOP_CONNECTION_FILE"

const remoteProtocolVersion = 1

type remoteSession struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Repo  string `json:"repo"`
	State string `json:"state"`
}

type remoteSessionsResponse struct {
	ProtocolVersion     int             `json:"protocolVersion"`
	TerminalWireVersion string          `json:"terminalWireVersion"`
	Sessions            []remoteSession `json:"sessions"`
}

func (ctrl *Controller) RemoteSessions(w http.ResponseWriter, r *http.Request) error {
	if err := requireTerminalToken(r, ctrl.terminalToken); err != nil {
		return err
	}
	if err := ctrl.core.Terminals.Available(r.Context()); err != nil {
		return err
	}
	sessions, err := ctrl.core.Sessions.ListSessions(r.Context())
	if err != nil {
		return err
	}
	out := remoteSessionsResponse{
		ProtocolVersion:     remoteProtocolVersion,
		TerminalWireVersion: terminalWireVersion,
		Sessions:            make([]remoteSession, 0, len(sessions)),
	}
	for _, s := range sessions {
		out.Sessions = append(out.Sessions, remoteSession{ID: s.ID, Name: s.Name, Slug: s.Slug, Repo: s.Remote, State: string(s.State)})
	}
	return server.JSON(w, http.StatusOK, out)
}

func (ctrl *Controller) RemoteSessionOptions(w http.ResponseWriter, r *http.Request) error {
	if err := requireTerminalToken(r, ctrl.terminalToken); err != nil {
		return err
	}
	options, err := ctrl.core.Sessions.SessionLaunchOptions(r.Context())
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, options)
}

type remoteCreateSessionRequest struct {
	Repository string `json:"repository"`
	Name       string `json:"name"`
	Prompt     string `json:"prompt"`
	Agent      string `json:"agent"`
}
type remoteCreateSessionResponse struct {
	JobID int64 `json:"jobId"`
}

func (ctrl *Controller) RemoteCreateSession(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[remoteCreateSessionRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	if body.Repository == "" {
		return app.Errorf(app.KindInvalid, "repository is required")
	}
	id, err := ctrl.core.Sessions.CreateSession(r.Context(), dispatch.CreateSessionRequest{
		Repository: body.Repository, Name: body.Name, Prompt: body.Prompt, Agent: body.Agent,
	})
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusAccepted, remoteCreateSessionResponse{JobID: id})
}

type remoteJobRequest struct {
	ID int64 `json:"id"`
}

func (ctrl *Controller) RemoteJob(w http.ResponseWriter, r *http.Request) error {
	body, err := terminalBody[remoteJobRequest](ctrl, w, r)
	if err != nil {
		return err
	}
	job, err := ctrl.core.Jobs.Get(r.Context(), body.ID)
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, job)
}

func (ctrl *Controller) RemoteSessionStatuses(w http.ResponseWriter, r *http.Request) error {
	if err := requireTerminalToken(r, ctrl.terminalToken); err != nil {
		return err
	}
	snapshot, err := ctrl.core.Sessions.SessionStatuses(r.Context())
	if err != nil {
		return err
	}
	return server.JSON(w, http.StatusOK, snapshot)
}
