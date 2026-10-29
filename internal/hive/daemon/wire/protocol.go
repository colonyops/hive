package wire

import "fmt"

// ProtocolVersion is the hive.daemon.v1 protocol this build speaks. Raise it
// when a change would break a client or a daemon of the previous version, and
// raise MinProtocolVersion when this build can no longer talk to an older
// daemon.
const (
	ProtocolVersion    = 1
	MinProtocolVersion = 1
)

// IncompatibleError is a daemon whose protocol this client does not support.
type IncompatibleError struct {
	DaemonProtocol int
	DaemonVersion  string
	ClientVersion  string
}

func (e *IncompatibleError) Error() string {
	return fmt.Sprintf("daemon hive %s speaks protocol %d; this hive %s supports protocols %d to %d; install the same hive version on both machines",
		orUnknown(e.DaemonVersion), e.DaemonProtocol, orUnknown(e.ClientVersion), MinProtocolVersion, ProtocolVersion)
}

// CheckProtocol returns an *IncompatibleError for a daemon protocol outside
// MinProtocolVersion to ProtocolVersion.
func CheckProtocol(daemonProtocol int, daemonVersion, clientVersion string) error {
	if daemonProtocol < MinProtocolVersion || daemonProtocol > ProtocolVersion {
		return &IncompatibleError{DaemonProtocol: daemonProtocol, DaemonVersion: daemonVersion, ClientVersion: clientVersion}
	}
	return nil
}

func orUnknown(version string) string {
	if version == "" {
		return "(unknown version)"
	}
	return version
}
