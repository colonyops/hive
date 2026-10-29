package session

import (
	"fmt"
	"regexp"
	"strings"
)

// LocalServer is the server name of this machine in a Ref. No entry under
// servers: may use it.
const LocalServer = "local"

var serverNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// ValidateServerName checks a name under servers:. The name must not hold
// the Ref separator and must not be LocalServer.
func ValidateServerName(name string) error {
	switch {
	case name == LocalServer:
		return fmt.Errorf("%q names this machine; choose another name", LocalServer)
	case !serverNamePattern.MatchString(name):
		return fmt.Errorf("%q: use letters, digits, and _ . - only", name)
	}
	return nil
}

// Ref names one session among every server a client works with. A session
// id is unique only on its own server.
type Ref struct {
	Server string
	ID     string
}

func LocalRef(id string) Ref {
	return Ref{Server: LocalServer, ID: id}
}

// ParseRef reads "<server>:<id>", or a bare id for a session on
// defaultServer.
func ParseRef(s, defaultServer string) (Ref, error) {
	server, id, found := strings.Cut(s, ":")
	if !found {
		server, id = defaultServer, s
	}
	switch {
	case id == "":
		return Ref{}, fmt.Errorf("session reference %q has no session id", s)
	case server == "":
		return Ref{}, fmt.Errorf("session reference %q has no server name", s)
	}
	return Ref{Server: server, ID: id}, nil
}

func (r Ref) IsLocal() bool {
	return r.Server == LocalServer
}

// String is the form ParseRef reads. A local session prints as its bare id,
// so local output does not change.
func (r Ref) String() string {
	if r.IsLocal() {
		return r.ID
	}
	return r.Server + ":" + r.ID
}
