package wailsui

import (
	"regexp"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// ShortCommit trims a git revision to its conventional 7-character short form,
// leaving shorter values (e.g. the "HEAD" default) untouched.
func ShortCommit(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

// releaseVersionRE matches the closed set of publishable versions enforced by
// cmd/release: X.Y.Z with an optional -dev.N / -beta.N
// prerelease identifier.
var releaseVersionRE = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-(dev|beta)\.[0-9]+)?$`)

// ReleaseChannel maps a published desktop version to its release channel
// (docs/decisions/0004): bare X.Y.Z → stable, X.Y.Z-beta.N → beta,
// X.Y.Z-dev.N → dev. ok is false for anything else — source builds ("dev"),
// "(devel)", pseudo-versions, and foreign prerelease identifiers — which also
// marks the version as unreleased.
func ReleaseChannel(version string) (channel string, ok bool) {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	m := releaseVersionRE.FindStringSubmatch(v)
	if m == nil {
		return "", false
	}
	switch m[1] {
	case "dev":
		return settings.ChannelDev, true
	case "beta":
		return settings.ChannelBeta, true
	default:
		return settings.ChannelStable, true
	}
}
