package wire

import "fmt"

// ProtocolVersion is the hive.daemon.v1 protocol this build speaks.
// MinProtocolVersion is the oldest daemon protocol it still works with. Most
// releases change neither, so client and server can differ in version. To cut
// off older daemons, for a change this client cannot work around, raise both
// in that release: only that release forces an upgrade. A new field or RPC
// that an older peer can ignore raises neither.
//
// Protocol 0 is a daemon from before Hello carried the field.
const (
	ProtocolVersion    = 1
	MinProtocolVersion = 0
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
