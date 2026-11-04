package daemonclient

import (
	"strings"

	"golang.org/x/mod/semver"
)

// releaseVersion returns the canonical semver of a release build, and false
// for `dev`, a prerelease, or anything else that is not X.Y.Z.
func releaseVersion(version string) (string, bool) {
	v := "v" + strings.TrimPrefix(strings.TrimSpace(version), "v")
	core, _, _ := strings.Cut(v, "+")
	if strings.Count(core, ".") != 2 || !semver.IsValid(v) || semver.Prerelease(v) != "" {
		return "", false
	}
	return semver.Canonical(v), true
}

// olderRelease reports whether both versions are releases and the daemon's is
// the lower. Any other pairing is not a reason to restart a daemon.
func olderRelease(daemon, client string) bool {
	d, ok := releaseVersion(daemon)
	if !ok {
		return false
	}
	c, ok := releaseVersion(client)
	return ok && semver.Compare(d, c) < 0
}
