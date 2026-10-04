// Package buildinfo reports which build of a hive program is running.
package buildinfo

import "runtime/debug"

// devVersion is the version a binary reports when no -ldflags stamped one.
const devVersion = "dev"

// Info is a binary's release identity.
type Info struct {
	Version string
	Commit  string
	Date    string
}

// Resolve returns the identity a program's -ldflags stamped. A binary built
// without them (`go install module@version`, `go build`, `go run`) still has
// version "dev"; Resolve then reads the module version and VCS metadata the
// Go toolchain records, keeping each stamped value the toolchain has no
// substitute for.
func Resolve(version, commit, date string) Info {
	vcs, ok := read()
	return resolve(Info{Version: version, Commit: commit, Date: date}, vcs, ok)
}

func resolve(info Info, vcs VCS, stamped bool) Info {
	if info.Version != devVersion || !stamped {
		return info
	}
	if vcs.ModuleVersion != "" && vcs.ModuleVersion != "(devel)" {
		info.Version = vcs.ModuleVersion
	}
	if vcs.Revision != "" {
		info.Commit = vcs.Revision
	}
	if vcs.Time != "" {
		info.Date = vcs.Time
	}
	return info
}

// VCS is what the Go toolchain stamps into a binary. Revision is empty under
// -buildvcs=false.
type VCS struct {
	ModuleVersion string
	Revision      string
	Modified      bool
	Time          string
	GoVersion     string
}

// ReadVCS reads the toolchain's stamp from the running binary. It is the zero
// value when the binary carries no build information.
func ReadVCS() VCS {
	vcs, _ := read()
	return vcs
}

func read() (VCS, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return VCS{}, false
	}
	vcs := VCS{ModuleVersion: info.Main.Version, GoVersion: info.GoVersion}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			vcs.Revision = s.Value
		case "vcs.modified":
			vcs.Modified = s.Value == "true"
		case "vcs.time":
			vcs.Time = s.Value
		}
	}
	return vcs, true
}
