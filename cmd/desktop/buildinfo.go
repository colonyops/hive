package main

// Build information for the desktop app. Populated at build time via
// -ldflags "-X main.version=... -X main.commit=... -X main.date=...". The
// production builds in cmd/desktop/build/darwin/Taskfile.yml and
// cmd/desktop/build/linux/Taskfile.yml, driven by the release pipeline
// (cmd/tools/release), stamp the release version, commit SHA, and build date here so
// the running app can report exactly what it is.
//
// A plain source build reports "dev".
var (
	version = "dev"
	commit  = "HEAD"
	date    = "now"
)
