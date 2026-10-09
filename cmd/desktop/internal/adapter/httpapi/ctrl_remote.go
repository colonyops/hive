package httpapi

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/hay-kot/httpkit/server"

	"github.com/colonyops/hive/pkg/atomicfile"
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

// WriteConnectionFile exports a per-run credential for retrieval through SSH.
// The parent directory must exist; atomic replacement never follows an old symlink.
func WriteConnectionFile(path, host string, port int, token string) error {
	data, err := json.Marshal(struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}{URL: "http://" + net.JoinHostPort(host, strconv.Itoa(port)), Token: token})
	if err != nil {
		return fmt.Errorf("encode remote connection: %w", err)
	}
	if err := atomicfile.Write(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write remote connection file: %w", err)
	}
	return nil
}
