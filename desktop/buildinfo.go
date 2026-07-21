package main

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// Build information for the desktop app. Populated at build time via
// -ldflags "-X main.version=... -X main.commit=... -X main.date=...". The
// production build in desktop/build/darwin/Taskfile.yml and the
// desktop-publish workflow stamp the release version, commit SHA, and build
// date here so the running app can report exactly what it is.
//
// Defaults mirror the CLI's (see the repository-root main.go): a plain source
// build reports "dev".
var (
	version = "dev"
	commit  = "HEAD"
	date    = "now"
)

// desktopRepoSlug is the GitHub owner/repo the desktop app is released from.
// Desktop releases live in their own tag namespace, desktop-v<version>, decoupled
// from the CLI's v<version> tags (see .github/workflows/desktop-publish.yml).
const desktopRepoSlug = "colonyops/hive"

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

// shortCommit trims a git revision to its conventional 7-character short form,
// leaving shorter values (e.g. the "HEAD" default) untouched.
func shortCommit(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

// repoURL is the desktop app's GitHub repository home page. Unlike releaseURL
// it is always available, so the About screen can always link to the project.
func repoURL() string {
	return fmt.Sprintf("https://github.com/%s", desktopRepoSlug)
}

// releaseURL returns the GitHub release page for a desktop version, or "" when
// the version has no corresponding published release: dev builds, empty
// values, and go-module pseudo-versions (v0.0.0-<time>-<sha>) all yield "".
// Desktop releases are tagged desktop-v<version>.
func releaseURL(version string) string {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if !isReleaseVersion(v) {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/releases/tag/desktop-v%s", desktopRepoSlug, v)
}

// isReleaseVersion reports whether v is a plain released semver core
// (major.minor.patch, digits only). This intentionally rejects pseudo-versions
// and "(devel)" so we never link to a release tag that does not exist.
func isReleaseVersion(v string) bool {
	nums := strings.Split(v, ".")
	if len(nums) != 3 {
		return false
	}
	for _, n := range nums {
		if n == "" {
			return false
		}
		for _, r := range n {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}
