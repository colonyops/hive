package wailsui

import (
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// DevToolsService exposes the developer-tools gate and two calls that exist
// only to be timed from the frontend (ADR developer-tools-are-reachable-in-a-shipped-build-behind-a-setting).
type DevToolsService struct {
	devtools *app.DevToolsService
}

func NewDevToolsService(devtools *app.DevToolsService) *DevToolsService {
	return &DevToolsService{devtools: devtools}
}

// DevToolsInfo is what the frontend needs before it decides to render the pane.
type DevToolsInfo struct {
	Enabled bool `json:"enabled"`
}

// Info reports whether the developer tools are reachable in this build.
func (s *DevToolsService) Info() DevToolsInfo {
	return DevToolsInfo{Enabled: s.devtools.Enabled()}
}

// Ping returns the service's clock in unix milliseconds. It does nothing else
// on purpose: timed from the frontend it prices one empty Wails round trip,
// and the returned instant splits that trip into its inbound and outbound legs.
func (s *DevToolsService) Ping() int64 { return time.Now().UnixMilli() }

// maxEchoBytes caps the payload Echo will build. Large enough to price a
// terminal-sized frame, small enough that a typo in the caller cannot ask the
// process for a gigabyte.
const maxEchoBytes = 4 << 20

// Echo returns a payload of the requested size so the panel can separate the
// fixed cost of a Wails call from the cost of marshalling what it carries.
func (s *DevToolsService) Echo(bytes int) string {
	if bytes <= 0 {
		return ""
	}
	return strings.Repeat("x", min(bytes, maxEchoBytes))
}
