package main

import "runtime/debug"

// Build information for the desktop app. Populated at build time via
// -ldflags "-X main.version=... -X main.commit=... -X main.date=...". The
// production builds in desktop/build/darwin/Taskfile.yml and
// desktop/build/linux/Taskfile.yml, driven by the release pipeline
// (cmd/release), stamp the release version, commit SHA, and build date here so
// the running app can report exactly what it is.
//
// A plain source build reports "dev".
var (
	version = "dev"
	commit  = "HEAD"
	date    = "now"
)

// resolvedBuildInfo returns the effective version, commit, and date for the
// running binary. When ldflags were not supplied (a plain `go build`/`go run`,
// where version is still "dev") it falls back to the module + VCS metadata Go
// records automatically, mirroring the CLI's resolvedBuildInfo.
func resolvedBuildInfo() (v, c, d string) {
	v, c, d = version, commit, date
	if v != "dev" {
		return v, c, d
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v, c, d
	}
	if mv := info.Main.Version; mv != "" && mv != "(devel)" {
		v = mv
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			c = s.Value
		case "vcs.time":
			d = s.Value
		}
	}
	return v, c, d
}
