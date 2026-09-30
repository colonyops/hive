package main

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var versionPattern = regexp.MustCompile(`^(?:desktop-v)?([0-9]+)\.([0-9]+)\.([0-9]+)(?:-(dev|beta)\.([0-9]+))?$`)

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

func validChannel(channel string) bool {
	return channel == "dev" || channel == "beta" || channel == "stable"
}

func parsePublishVersion(value string) (releaseVersion, error) {
	version, err := parseVersion(value)
	if err != nil {
		return releaseVersion{}, err
	}
	if strings.TrimSpace(value) != version.String() {
		return releaseVersion{}, errors.New("expected version without desktop-v prefix")
	}
	return version, nil
}

func parseVersion(value string) (releaseVersion, error) {
	match := versionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return releaseVersion{}, errors.New("expected X.Y.Z or X.Y.Z-(dev|beta).N")
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

func (v releaseVersion) channel() string {
	if v.prerelease == "" {
		return "stable"
	}
	return v.prerelease
}

func (v releaseVersion) affectedChannels() []string {
	switch v.channel() {
	case "stable":
		return []string{"stable", "beta", "dev"}
	case "beta":
		return []string{"beta", "dev"}
	default:
		return []string{"dev"}
	}
}

// compareChannelRelease orders versions by base version and then by the
// product's channel promotion path: dev, beta, stable. SemVer orders the words
// "beta" and "dev" lexically, which is the opposite of this product's release
// progression and would reject an intentional dev-to-beta promotion.
func compareChannelRelease(left, right releaseVersion) int {
	if result := compareBase(left.base, right.base); result != 0 {
		return result
	}
	rank := func(version releaseVersion) int {
		switch version.channel() {
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

func nextVersion(channel string, versions []releaseVersion) string {
	base := baseVersion{minor: 1}
	if len(versions) > 0 {
		base = versions[0].base
		for _, version := range versions[1:] {
			if compareBase(version.base, base) > 0 {
				base = version.base
			}
		}
	}

	var existing []releaseVersion
	for _, version := range versions {
		if compareBase(version.base, base) == 0 {
			existing = append(existing, version)
		}
	}

	hasStable := false
	hasBeta := false
	for _, version := range existing {
		hasStable = hasStable || version.prerelease == ""
		hasBeta = hasBeta || version.prerelease == "beta"
	}

	suffix := ""
	switch channel {
	case "dev":
		if hasStable || hasBeta {
			base.patch++
			suffix = "-dev.1"
		} else {
			suffix = fmt.Sprintf("-dev.%d", nextPrereleaseNumber(existing, "dev"))
		}
	case "beta":
		if hasStable {
			base.patch++
			suffix = "-beta.1"
		} else {
			suffix = fmt.Sprintf("-beta.%d", nextPrereleaseNumber(existing, "beta"))
		}
	case "stable":
		if hasStable {
			base.patch++
		}
	}
	return fmt.Sprintf("%d.%d.%d%s", base.major, base.minor, base.patch, suffix)
}

func nextPrereleaseNumber(versions []releaseVersion, prerelease string) int {
	maximum := 0
	for _, version := range versions {
		if version.prerelease == prerelease && version.number > maximum {
			maximum = version.number
		}
	}
	return maximum + 1
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
