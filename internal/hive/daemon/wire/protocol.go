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

// CatalogVersion is the revision of the operation catalog this build serves
// at the daemon's /mcp and calls there. MinCatalogVersion is the oldest it
// still calls. Raise CatalogVersion for a change that breaks an older peer's
// engine calls, such as a renamed tool or a changed field meaning, and
// MinCatalogVersion too when this client cannot work around it. A new tool or
// field raises neither.
//
// Catalog 0 is a daemon that serves no engine operations. Its session calls
// still work: the catalog range does not gate them.
const (
	CatalogVersion    = 1
	MinCatalogVersion = 1
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

// EnginePath is where a daemon serves the engine operations of the catalog,
// over MCP on the same connection as its Connect services.
const EnginePath = "/mcp"

// CatalogError is a daemon whose catalog this client does not call.
type CatalogError struct {
	DaemonCatalog int
	DaemonVersion string
	ClientVersion string
}

func (e *CatalogError) Error() string {
	if e.DaemonCatalog == 0 {
		return fmt.Sprintf("daemon hive %s serves no engine operations; this hive %s calls catalog %d to %d; install the same hive version on both machines",
			orUnknown(e.DaemonVersion), orUnknown(e.ClientVersion), MinCatalogVersion, CatalogVersion)
	}
	return fmt.Sprintf("daemon hive %s serves operation catalog %d; this hive %s calls catalog %d to %d; install the same hive version on both machines",
		orUnknown(e.DaemonVersion), e.DaemonCatalog, orUnknown(e.ClientVersion), MinCatalogVersion, CatalogVersion)
}

// CheckCatalog returns a *CatalogError for a daemon catalog outside
// MinCatalogVersion to CatalogVersion.
func CheckCatalog(daemonCatalog int, daemonVersion, clientVersion string) error {
	if daemonCatalog < MinCatalogVersion || daemonCatalog > CatalogVersion {
		return &CatalogError{DaemonCatalog: daemonCatalog, DaemonVersion: daemonVersion, ClientVersion: clientVersion}
	}
	return nil
}

func orUnknown(version string) string {
	if version == "" {
		return "(unknown version)"
	}
	return version
}
