package wailsui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// DefaultManifestBaseURL is the public download domain fronting the release
// bucket (docs/distribution.md). Decision 0003 makes the fronting domain the
// hard requirement: binaries bake in dl.hivedesktop.com, never a raw bucket
// URL, so storage stays swappable without an app update.
const DefaultManifestBaseURL = "https://dl.hivedesktop.com"

// artifactURLKey carries the manifest's artifact URL from Check to Download in
// the Release metadata.
const artifactURLKey = "manifest.artifact.url"

// manifestProvider implements updater.Provider against the channel manifests
// published by cmd/release: GET
// <base>/desktop/channels/<channel>/latest.json, compare semver, download the
// manifest's artifact URL, and let the Updater verify the manifest's sha256
// (decisions 0003/0004; schema in docs/distribution.md). The provider holds no
// credentials and never lists the bucket.
type manifestProvider struct {
	base    string // fronting domain, no trailing slash
	channel string // one of settings.ChannelStable/ChannelBeta/ChannelDev
	client  *http.Client
}

// NewManifestProvider builds a provider polling channel under base.
func NewManifestProvider(base, channel string) *manifestProvider {
	return &manifestProvider{
		base:    strings.TrimSuffix(base, "/"),
		channel: channel,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Name implements updater.Provider.
func (p *manifestProvider) Name() string { return "manifest" }

// Check implements updater.Provider. It fetches the channel manifest and
// returns a Release when the manifest's version is newer than the running
// version. Returns (nil, nil) when up to date or when the channel has no
// published release yet.
func (p *manifestProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	m, err := p.fetchManifest(ctx)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}
	// Defense-in-depth per decision 0004: the manifest states which channel it
	// was written for; a mismatch means a misrouted publish or a swapped file.
	if m.Channel != p.channel {
		return nil, fmt.Errorf("desktop updater: manifest channel %q does not match configured channel %q", m.Channel, p.channel)
	}

	latest := canonicalSemver(m.Version)
	if !semver.IsValid(latest) {
		return nil, fmt.Errorf("desktop updater: manifest version %q is not valid semver", m.Version)
	}
	current := canonicalSemver(strings.TrimPrefix(strings.TrimPrefix(req.CurrentVersion, "desktop-v"), "v"))
	if !semver.IsValid(current) || semver.Compare(latest, current) <= 0 {
		return nil, nil
	}

	key := platformKey(req.Platform, req.Arch)
	entry, ok := m.Platforms[key]
	if !ok {
		return nil, fmt.Errorf("desktop updater: manifest %s@%s has no artifact for platform %q", m.Channel, m.Version, key)
	}
	digest, err := hex.DecodeString(entry.SHA256)
	if err != nil || len(digest) != sha256.Size {
		return nil, fmt.Errorf("desktop updater: manifest sha256 for platform %q is malformed", key)
	}
	artifactURL, err := url.Parse(entry.URL)
	if err != nil {
		return nil, fmt.Errorf("desktop updater: manifest artifact URL: %w", err)
	}

	return &updater.Release{
		Version: strings.TrimPrefix(latest, "v"),
		Channel: m.Channel,
		// A pending release is not installed, so its notes cannot come from the
		// embedded changelog — the manifest is the only place they exist yet.
		Notes:       manifestNotes(m),
		PublishedAt: m.PubDate,
		Artifact: updater.Artifact{
			Filename: path.Base(artifactURL.Path),
			Filetype: "zip",
			Size:     entry.Size,
			Platform: req.Platform,
			Arch:     req.Arch,
		},
		Verification: &updater.Verification{DigestAlgo: "sha256", Digest: digest},
		Metadata:     map[string]any{artifactURLKey: entry.URL},
	}, nil
}

// Download implements updater.Provider by streaming the artifact URL stashed
// in the Release metadata. Integrity comes from the Updater checking the
// manifest's sha256 against the streamed bytes, not from the URL's host.
func (p *manifestProvider) Download(ctx context.Context, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	if rel == nil || rel.Metadata == nil {
		return errors.New("desktop updater: release missing metadata (was it produced by this provider?)")
	}
	urlStr, ok := rel.Metadata[artifactURLKey].(string)
	if !ok || urlStr == "" {
		return errors.New("desktop updater: release metadata missing artifact URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("desktop updater: download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("desktop updater: download HTTP %d", resp.StatusCode)
	}

	total := rel.Artifact.Size
	if total == 0 && resp.ContentLength > 0 {
		total = resp.ContentLength
	}
	var written int64
	buf := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if onProgress != nil {
				onProgress(written, total)
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// fetchManifest GETs the channel's latest.json. A 404 means the channel has no
// published release yet and yields (nil, nil).
func (p *manifestProvider) fetchManifest(ctx context.Context) (*channelManifest, error) {
	endpoint := p.base + "/desktop/channels/" + p.channel + "/latest.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("desktop updater: fetch manifest: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, nil
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("desktop updater: manifest HTTP %d: %s", resp.StatusCode, body)
	}
	var m channelManifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&m); err != nil {
		return nil, fmt.Errorf("desktop updater: decode manifest: %w", err)
	}
	return &m, nil
}

// manifestNotes prefers the full changelog body and falls back to the summary,
// so a release that ships only a one-liner still says something. Nothing
// renders the result yet — it reaches the frontend as UpdateInfo.Notes and
// stops there. A surface that shows it should read the manifest's summary and
// notes separately, the way every other release-notes path keeps them, rather
// than inherit this flattening.
func manifestNotes(m *channelManifest) string {
	if notes := strings.TrimSpace(m.Notes); notes != "" {
		return notes
	}
	return strings.TrimSpace(m.Summary)
}

// canonicalSemver returns a "v"-prefixed canonical form suitable for
// golang.org/x/mod/semver, which requires the leading "v".
func canonicalSemver(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// platformKey maps the runtime platform/arch to a manifest platforms key.
// macOS ships one universal build, so both darwin arches resolve to
// "darwin-universal"; other platforms use "<os>-<arch>".
func platformKey(platform, arch string) string {
	if platform == "darwin" {
		return "darwin-universal"
	}
	return platform + "-" + arch
}

// --- manifest schema (docs/distribution.md) ---

type channelManifest struct {
	Channel string    `json:"channel"`
	Version string    `json:"version"`
	PubDate time.Time `json:"pub_date"`
	// Summary and Notes carry the release's changelog entry — the only source
	// for a release that is not installed yet. Both are absent from manifests
	// published before the changelog existed.
	Summary   string                      `json:"summary"`
	Notes     string                      `json:"notes"`
	Platforms map[string]manifestPlatform `json:"platforms"`
}

type manifestPlatform struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
