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

// ValidateServerName checks a name under servers:. A name is letters,
// digits, and _ . -, starts with a letter or digit, and is not LocalServer,
// so a "<server>:<id>" Ref stays one plain argument on a command line and in
// a hive:// link.
func ValidateServerName(name string) error {
	switch {
	case name == LocalServer:
		return fmt.Errorf("%q names this machine; choose another name", LocalServer)
	case !serverNamePattern.MatchString(name):
		return fmt.Errorf("%q: start with a letter or digit, then letters, digits, _ . -", name)
	}
	return nil
}

// Ref names one session among every server a client works with. A session
// id is unique only on its own server.
type Ref struct {
	Server string
	ID     string
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

// String is the form ParseRef reads. A local session prints as its bare id,
// so local output does not change.
func (r Ref) String() string {
	if r.Server == LocalServer {
		return r.ID
	}
	return r.Server + ":" + r.ID
}
