package main

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// versionPattern reads every version this repository has published: the CLI's
// v* tags, the desktop's desktop-v* tags, and the dev and beta prereleases the
// desktop channels carried before every program shared one version. Only a
// bare X.Y.Z is publishable now (parsePublishVersion).
var versionPattern = regexp.MustCompile(`^(?:desktop-)?v?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-(dev|beta)\.([0-9]+))?$`)

// manifestChannels are the desktop update manifests every release writes.
// There is one release line, but installs that follow the beta or dev
// manifest still exist, and they converge only if those manifests carry the
// same release (ADR every-program-ships-under-one-shared-version).
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

type bumpLevel string

const (
	bumpPatch bumpLevel = "patch"
	bumpMinor bumpLevel = "minor"
	bumpMajor bumpLevel = "major"
)

func parseBumpLevel(value string) (bumpLevel, error) {
	switch level := bumpLevel(value); level {
	case bumpPatch, bumpMinor, bumpMajor:
		return level, nil
	default:
		return "", fmt.Errorf("unknown bump level %q: expected patch, minor, or major", value)
	}
}

// nextVersion bumps the newest published version of any program. Every
// program shares the version, so the CLI's v0.59.0 and the desktop's 0.9.x
// both count, and the next release advances past all of them.
func nextVersion(published []releaseVersion, level bumpLevel) releaseVersion {
	newest, _ := newestVersion(published)
	base := newest.base
	switch level {
	case bumpMajor:
		return releaseVersion{base: baseVersion{major: base.major + 1}}
	case bumpMinor:
		return releaseVersion{base: baseVersion{major: base.major, minor: base.minor + 1}}
	default:
		// A prerelease base has not shipped as itself, but the release line
		// has no prereleases now, so its next patch is still above it.
		return releaseVersion{base: baseVersion{major: base.major, minor: base.minor, patch: base.patch + 1}}
	}
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
