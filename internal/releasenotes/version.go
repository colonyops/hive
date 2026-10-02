package releasenotes

import (
	"regexp"
	"strconv"
	"strings"
)

// versionRE matches every version a release has published: X.Y.Z, and the
// -dev.N and -beta.N prereleases the desktop channels carried before every
// program shared one version. The acknowledged version of an install that
// predates the change is still one of those.
var versionRE = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)(?:-(dev|beta)\.([0-9]+))?$`)

// version is a published version decomposed for ordering.
type version struct {
	major      int
	minor      int
	patch      int
	prerelease string // "", "beta", or "dev"
	number     int
}

// parseVersion accepts a version with or without the desktop-v / v prefixes.
// ok is false for anything outside the publishable set — source builds
// ("dev"), "(devel)", and go module pseudo-versions among them — which is also
// how callers detect a build that has no release to describe.
func parseVersion(s string) (version, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "desktop-")
	s = strings.TrimPrefix(s, "v")
	m := versionRE.FindStringSubmatch(s)
	if m == nil {
		return version{}, false
	}
	parts := make([]int, 4)
	for i, raw := range []string{m[1], m[2], m[3], m[5]} {
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			return version{}, false
		}
		parts[i] = n
	}
	if m[4] != "" && parts[3] < 1 {
		return version{}, false
	}
	return version{major: parts[0], minor: parts[1], patch: parts[2], prerelease: m[4], number: parts[3]}, true
}

// IsPublished reports whether version names a release this project publishes.
// It is false for source builds ("dev"), "(devel)", go module pseudo-versions
// and foreign prerelease identifiers — builds with no release to describe.
func IsPublished(version string) bool {
	_, ok := parseVersion(version)
	return ok
}

// IsNewer reports whether a is a later release than b. An unparseable a is
// never newer — a source build has not "upgraded" from anything — while an
// unparseable b means nothing has been recorded to compare against.
func IsNewer(a, b string) bool {
	left, ok := parseVersion(a)
	if !ok {
		return false
	}
	right, ok := parseVersion(b)
	if !ok {
		return true
	}
	return compare(left, right) > 0
}

// channelRank orders a version's prerelease identifier along this product's
// promotion path. SemVer sorts the words "beta" and "dev" lexically, which is
// the reverse of how a build is promoted here, so ordering goes through this
// rank instead (ADR release-channels).
func channelRank(prerelease string) int {
	switch prerelease {
	case "dev":
		return 0
	case "beta":
		return 1
	default:
		return 2
	}
}

// compare orders two versions by base version, then by promotion rank, then by
// prerelease number.
func compare(a, b version) int {
	for _, pair := range [][2]int{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] != pair[1] {
			return sign(pair[0] - pair[1])
		}
	}
	if ra, rb := channelRank(a.prerelease), channelRank(b.prerelease); ra != rb {
		return sign(ra - rb)
	}
	return sign(a.number - b.number)
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}
