package main

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// versionPattern reads every version this repository has published: the CLI's
// v* tags, the desktop's desktop-v* tags, and the dev and beta prereleases the
// desktop channels carried before every program shared one version. Only a
// bare X.Y.Z is publishable now (parsePublishVersion).
var versionPattern = regexp.MustCompile(`^(?:desktop-)?v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-(dev|beta)\.([0-9]+))?$`)

// manifestChannels are the desktop update manifests every release writes.
// There is one release line, but installs that follow the beta or dev
// manifest still exist, and they converge only if those manifests carry the
// same release (ADR every-program-ships-under-one-date-based-version).
var manifestChannels = []string{"stable", "beta", "dev"}

type baseVersion struct {
	major int
	minor int
	patch int
}

type releaseVersion struct {
	base       baseVersion
	prerelease string
	number     int
}

// parsePublishVersion accepts a version a release can publish: X.Y.Z, with no
// tag prefix and no prerelease.
func parsePublishVersion(value string) (releaseVersion, error) {
	version, err := parseVersion(value)
	if err != nil {
		return releaseVersion{}, err
	}
	if version.prerelease != "" {
		return releaseVersion{}, fmt.Errorf("%s is a prerelease: every release is X.Y.Z", version)
	}
	if strings.TrimSpace(value) != version.String() {
		return releaseVersion{}, errors.New("expected a version without a v or desktop-v prefix")
	}
	return version, nil
}

func parseVersion(value string) (releaseVersion, error) {
	match := versionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return releaseVersion{}, errors.New("expected X.Y.Z")
	}

	parts := make([]int, 4)
	for i, raw := range []string{match[1], match[2], match[3], match[5]} {
		if raw == "" {
			continue
		}
		part, err := strconv.Atoi(raw)
		if err != nil {
			return releaseVersion{}, fmt.Errorf("numeric component %q: %w", raw, err)
		}
		parts[i] = part
	}
	if match[4] != "" && parts[3] < 1 {
		return releaseVersion{}, errors.New("prerelease number must be at least 1")
	}
	return releaseVersion{
		base:       baseVersion{major: parts[0], minor: parts[1], patch: parts[2]},
		prerelease: match[4],
		number:     parts[3],
	}, nil
}

func (v releaseVersion) String() string {
	base := fmt.Sprintf("%d.%d.%d", v.base.major, v.base.minor, v.base.patch)
	if v.prerelease == "" {
		return base
	}
	return fmt.Sprintf("%s-%s.%d", base, v.prerelease, v.number)
}

// tag is the git tag of a release. Every program shares it: the CLI's go
// install and Homebrew cask read it, and the desktop's release records it.
func (v releaseVersion) tag() string { return "v" + v.String() }

// compareVersions orders versions by base version and then along the
// promotion path the desktop channels used: dev, beta, stable. SemVer orders
// the words "beta" and "dev" lexically, which is the reverse, and the release
// history still holds both.
func compareVersions(left, right releaseVersion) int {
	if result := compareBase(left.base, right.base); result != 0 {
		return result
	}
	rank := func(version releaseVersion) int {
		switch version.prerelease {
		case "dev":
			return 0
		case "beta":
			return 1
		default:
			return 2
		}
	}
	if leftRank, rightRank := rank(left), rank(right); leftRank != rightRank {
		if leftRank < rightRank {
			return -1
		}
		return 1
	}
	switch {
	case left.number < right.number:
		return -1
	case left.number > right.number:
		return 1
	default:
		return 0
	}
}

// dateVersionLayout is the minor component of a release version.
const dateVersionLayout = "20060102"

// nextVersion is the version a release cut at now takes: 0.YYYYMMDD.N, with
// the UTC date and N counting that day's releases from 0.
//
// The major version stays 0 on purpose. Go requires a /vN module path for a
// major version of 2 or more, so a year in the major component would leave
// `go install github.com/colonyops/hive@latest` on the last v0 release
// forever (ADR every-program-ships-under-one-date-based-version).
func nextVersion(now time.Time, published []releaseVersion) (releaseVersion, error) {
	date, err := strconv.Atoi(now.UTC().Format(dateVersionLayout))
	if err != nil {
		return releaseVersion{}, err
	}
	next := releaseVersion{base: baseVersion{minor: date}}
	for _, version := range published {
		if version.base.major == 0 && version.base.minor == date && version.base.patch >= next.base.patch {
			next.base.patch = version.base.patch + 1
		}
	}
	if newest, ok := newestVersion(published); ok && compareVersions(next, newest) <= 0 {
		return releaseVersion{}, fmt.Errorf("%s does not advance the newest published version %s; check the system clock", next, newest)
	}
	return next, nil
}

func newestVersion(versions []releaseVersion) (releaseVersion, bool) {
	if len(versions) == 0 {
		return releaseVersion{}, false
	}
	newest := versions[0]
	for _, version := range versions[1:] {
		if compareVersions(version, newest) > 0 {
			newest = version
		}
	}
	return newest, true
}

func compareBase(left, right baseVersion) int {
	for _, pair := range [][2]int{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}
