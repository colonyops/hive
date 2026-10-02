package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

const defaultDownloadBase = "https://dl.hivedesktop.com"

var errManifestNotFound = errors.New("manifest not found")

// platformManifest describes one platform's artifacts. url/sha256/size are the
// update artifact the in-app updater downloads; the installer_* fields are the
// human download, present only where the two differ (macOS ships a .dmg, and
// the updater cannot consume one). They are optional: manifests published
// before the installer existed have to keep parsing.
type platformManifest struct {
	URL             string `json:"url"`
	SHA256          string `json:"sha256"`
	Size            int64  `json:"size"`
	InstallerURL    string `json:"installer_url,omitempty"`
	InstallerSHA256 string `json:"installer_sha256,omitempty"`
	InstallerSize   int64  `json:"installer_size,omitempty"`
}

// channelManifest is the channel pointer the updater polls. summary and notes
// carry the version's changelog entry, which is the only place a pending
// release's notes exist — the app's own copy is embedded in the build it
// describes, and that build is not installed yet. Nothing renders them today;
// they reach the frontend as UpdateInfo.Notes and stop there. Both are
// optional, because manifests published before the changelog existed still
// have to parse.
type channelManifest struct {
	Channel   string                      `json:"channel"`
	Version   string                      `json:"version"`
	PubDate   string                      `json:"pub_date"`
	Summary   string                      `json:"summary,omitempty"`
	Notes     string                      `json:"notes,omitempty"`
	Platforms map[string]platformManifest `json:"platforms"`
}

func downloadBaseURL() string {
	if value := strings.TrimSpace(os.Getenv("DL_BASE_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}
	if value := strings.TrimSpace(os.Getenv("HIVE_DESKTOP_MANIFEST_BASE")); value != "" {
		value = strings.TrimRight(value, "/")
		return strings.TrimSuffix(value, "/desktop/channels")
	}
	return defaultDownloadBase
}

func manifestBaseURL() string {
	if value := strings.TrimSpace(os.Getenv("HIVE_DESKTOP_MANIFEST_BASE")); value != "" {
		return strings.TrimRight(value, "/")
	}
	return downloadBaseURL() + "/desktop/channels"
}

func fetchManifest(ctx context.Context, client *http.Client, url string) (channelManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return channelManifest{}, fmt.Errorf("create manifest request %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "hive-desktop-release/1")

	resp, err := client.Do(req)
	if err != nil {
		return channelManifest{}, fmt.Errorf("read %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return channelManifest{}, errManifestNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return channelManifest{}, fmt.Errorf("read %s: HTTP %d", url, resp.StatusCode)
	}

	var manifest channelManifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&manifest); err != nil {
		return channelManifest{}, fmt.Errorf("decode %s: %w", url, err)
	}
	if err := validateManifest(manifest); err != nil {
		return channelManifest{}, fmt.Errorf("decode %s: %w", url, err)
	}
	return manifest, nil
}

func validateManifest(manifest channelManifest) error {
	if manifest.Channel == "" || manifest.Version == "" || manifest.PubDate == "" || manifest.Platforms == nil {
		return errors.New("missing required manifest fields")
	}
	if _, err := time.Parse(time.RFC3339, manifest.PubDate); err != nil {
		return fmt.Errorf("invalid pub_date: %w", err)
	}
	// darwin-universal is required rather than the full platform set: manifests
	// published before Linux shipped carry only that key, and they still have to
	// parse for advancement checks against the live channels.
	if _, ok := manifest.Platforms["darwin-universal"]; !ok {
		return errors.New("missing darwin-universal artifact metadata")
	}
	for key, platform := range manifest.Platforms {
		if err := validatePlatformManifest(key, platform); err != nil {
			return err
		}
	}
	return nil
}

func validatePlatformManifest(key string, platform platformManifest) error {
	if err := validateArtifactMetadata(key, platform.URL, platform.SHA256, platform.Size); err != nil {
		return err
	}
	if platform.InstallerURL == "" && platform.InstallerSHA256 == "" && platform.InstallerSize == 0 {
		return nil
	}
	// Partial installer metadata is worse than none: a consumer that reads the
	// URL and skips the missing checksum would install unverified bytes.
	return validateArtifactMetadata(key+" installer", platform.InstallerURL, platform.InstallerSHA256, platform.InstallerSize)
}

func validateArtifactMetadata(label, artifact, checksum string, size int64) error {
	if artifact == "" || checksum == "" || size < 1 {
		return fmt.Errorf("missing %s artifact metadata", label)
	}
	artifactURL, err := url.Parse(artifact)
	if err != nil || artifactURL.Host == "" || (artifactURL.Scheme != "http" && artifactURL.Scheme != "https") {
		return fmt.Errorf("invalid %s artifact URL", label)
	}
	digest, err := hex.DecodeString(checksum)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("invalid %s SHA-256", label)
	}
	return nil
}

func readManifests(ctx context.Context) (map[string]channelManifest, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	manifests := make(map[string]channelManifest)
	for _, channel := range []string{"stable", "beta", "dev"} {
		manifest, err := fetchManifest(ctx, client, manifestBaseURL()+"/"+channel+"/latest.json")
		if errors.Is(err, errManifestNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if manifest.Channel != channel {
			return nil, fmt.Errorf("%s manifest declares channel %q", channel, manifest.Channel)
		}
		if _, err := parseVersion(manifest.Version); err != nil {
			return nil, fmt.Errorf("%s manifest version %q: %w", channel, manifest.Version, err)
		}
		manifests[channel] = manifest
	}
	return manifests, nil
}

func validateManifestAdvancement(candidate releaseVersion, manifests map[string]channelManifest) error {
	for _, channel := range manifestChannels {
		manifest, ok := manifests[channel]
		if !ok {
			continue
		}
		current, err := parseVersion(manifest.Version)
		if err != nil {
			return fmt.Errorf("%s manifest version %q: %w", channel, manifest.Version, err)
		}
		if compareVersions(candidate, current) <= 0 {
			return fmt.Errorf("candidate %s does not advance the %s manifest at %s", candidate, channel, manifest.Version)
		}
	}
	return nil
}

// releaseTagVersions reads every release tag: the shared v* tags and the
// desktop-v* tags the desktop used before it shared a version with the CLI.
// A tag the pattern matches but that is not a version, such as
// v2-experiment, is not a release and is skipped.
func releaseTagVersions(ctx context.Context) ([]releaseVersion, error) {
	output, err := exec.CommandContext(ctx, "git", "tag", "--list", "v[0-9]*", "desktop-v*").Output()
	if err != nil {
		return nil, fmt.Errorf("list release tags: %w", err)
	}

	var versions []releaseVersion
	for tag := range strings.Lines(string(output)) {
		version, err := parseVersion(strings.TrimSpace(tag))
		if err != nil {
			continue
		}
		versions = append(versions, version)
	}
	return versions, nil
}

func releaseVersions(ctx context.Context) ([]releaseVersion, map[string]channelManifest, error) {
	versions, err := releaseTagVersions(ctx)
	if err != nil {
		return nil, nil, err
	}
	manifests, err := readManifests(ctx)
	if err != nil {
		return nil, nil, err
	}
	for channel, manifest := range manifests {
		version, err := parseVersion(manifest.Version)
		if err != nil {
			return nil, nil, fmt.Errorf("%s manifest version %q: %w", channel, manifest.Version, err)
		}
		versions = append(versions, version)
	}
	return versions, manifests, nil
}

func verifyLive(ctx context.Context, version releaseVersion) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	var platforms map[string]platformManifest
	for _, channel := range manifestChannels {
		manifestURL := manifestBaseURL() + "/" + channel + "/latest.json"
		manifest, err := fetchManifest(ctx, client, manifestURL)
		if err != nil {
			return fmt.Errorf("verify %s manifest at %s: %w", channel, manifestURL, err)
		}
		if manifest.Channel != channel || manifest.Version != version.String() {
			return fmt.Errorf("%s manifest reports channel=%q version=%q, want channel=%q version=%q", channel, manifest.Channel, manifest.Version, channel, version.String())
		}
		// Every affected channel points at one release, so they must agree on
		// the whole platform set — not just on any single platform.
		if platforms != nil && !maps.Equal(platforms, manifest.Platforms) {
			return fmt.Errorf("%s manifest artifacts differ from the other affected manifests", channel)
		}
		platforms = manifest.Platforms
		fmt.Printf("manifest: %s -> %s (%d platforms)\n", channel, manifest.Version, len(manifest.Platforms))
	}

	// Sorted so output and failures are deterministic across runs.
	for _, key := range slices.Sorted(maps.Keys(platforms)) {
		entry := platforms[key]
		if err := verifyLiveArtifact(ctx, client, key, entry.URL, entry.SHA256, entry.Size); err != nil {
			return err
		}
		if entry.InstallerURL == "" {
			continue
		}
		if err := verifyLiveArtifact(ctx, client, key+" installer", entry.InstallerURL, entry.InstallerSHA256, entry.InstallerSize); err != nil {
			return err
		}
	}
	return nil
}

// verifyLiveArtifact downloads one published artifact and checks it against the
// size and checksum the manifest advertises — the same values the in-app updater
// will verify against, so a mismatch here is a broken update for that platform.
func verifyLiveArtifact(ctx context.Context, client *http.Client, label, artifactURL, checksum string, size int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactURL, nil)
	if err != nil {
		return fmt.Errorf("create %s artifact request: %w", label, err)
	}
	req.Header.Set("User-Agent", "hive-desktop-release/1")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s artifact: %w", label, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s artifact: HTTP %d", label, resp.StatusCode)
	}
	hash := sha256.New()
	n, err := io.Copy(hash, resp.Body)
	if err != nil {
		return fmt.Errorf("hash %s artifact: %w", label, err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != checksum {
		return fmt.Errorf("%s artifact checksum %s does not match manifest %s", label, actual, checksum)
	}
	if n != size {
		return fmt.Errorf("%s artifact size %d does not match manifest %d", label, n, size)
	}
	fmt.Printf("artifact: %s\n", artifactURL)
	fmt.Printf("  platform: %s\n", label)
	fmt.Printf("  size: %d\n", n)
	fmt.Printf("  sha256: %s\n", actual)
	return nil
}
