package process

// ProcessReader abstracts OS-level process introspection.
// Unit tests inject fakes; production uses OSReader.
type ProcessReader interface {
	// TPGID returns the terminal process group ID for pid.
	TPGID(pid int) (int, error)
	// Comm returns the short process name for pid.
	Comm(pid int) string
	// Cmdline returns the full argument list for pid.
	Cmdline(pid int) ([]string, error)
	// Environ returns the environment map for pid, or nil if unavailable.
	Environ(pid int) map[string]string
	// Children returns direct child PIDs of the given pid.
	// Used for depth-2 wrapper detection (e.g., node → claude).
	Children(pid int) ([]int, error)
}

// OSReader reads process info from the real OS.
type OSReader struct{}

// TPGID returns the terminal process group ID for pid.
func (OSReader) TPGID(pid int) (int, error) { return tpgidFromPID(pid) }

// Comm returns the short process name for pid.
func (OSReader) Comm(pid int) string { return commForPID(pid) }

// Cmdline returns the full argument list for pid.
func (OSReader) Cmdline(pid int) ([]string, error) { return cmdlineForPID(pid) }

// Environ returns the environment map for pid, or nil if unavailable.
func (OSReader) Environ(pid int) map[string]string { return environForPID(pid) }

// Children returns direct child PIDs of the given pid.
func (OSReader) Children(pid int) ([]int, error) { return childrenForPID(pid) }
