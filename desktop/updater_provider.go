package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"github.com/wailsapp/wails/v3/pkg/updater"
	ghprovider "github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

const (
	// desktopTagPrefix scopes the provider to the desktop release namespace.
	// CLI releases in the same repo are tagged v<version> and are ignored.
	desktopTagPrefix = "desktop-v"

	// checksumAssetName is the sidecar the release workflow uploads; the digest
	// for the picked zip is parsed from it and attached to the Release so the
	// Updater integrity-checks the download.
	checksumAssetName = "SHA256SUMS"

	// defaultGithubAPIBase is the public GitHub API root. Overridable for tests.
	defaultGithubAPIBase = "https://api.github.com"
)

// desktopProvider implements updater.Provider for colonyops/hive releases
// tagged "desktop-v<semver>". The stock github.New provider can't be used
// directly: it hits /releases/latest (which returns the newest release across
// the whole repo, possibly a CLI v* release) and its semver trimming leaves
// "desktop-v" intact, so a desktop release never registers as newer. This
// provider lists releases, filters to the desktop tag namespace, strips the
// prefix, and compares semver. Download is delegated to the stock provider,
// which only needs Metadata["github.asset.url"] — a value we control.
type desktopProvider struct {
	repo     string // "owner/repo"
	prefix   string // "desktop-v"
	token    string // optional PAT; empty for public repos
	base     string // API base URL
	client   *http.Client
	download *ghprovider.Provider // delegate for byte streaming
}

// newDesktopProvider builds a provider for repo ("owner/repo"). token may be
// empty for public repos.
func newDesktopProvider(repo, token string) (*desktopProvider, error) {
	if !strings.Contains(repo, "/") {
		return nil, fmt.Errorf("desktop updater: repo must be \"owner/repo\" (got %q)", repo)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	// The stock provider handles the redirect-to-presigned-URL download with
	// Authorization stripping; we reuse it verbatim for Download.
	dl, err := ghprovider.New(ghprovider.Config{
		Repository:    repo,
		Token:         token,
		ChecksumAsset: checksumAssetName,
		HTTPClient:    client,
	})
	if err != nil {
		return nil, fmt.Errorf("desktop updater: build download delegate: %w", err)
	}
	return &desktopProvider{
		repo:     repo,
		prefix:   desktopTagPrefix,
		token:    token,
		base:     defaultGithubAPIBase,
		client:   client,
		download: dl,
	}, nil
}

// Name implements updater.Provider.
func (p *desktopProvider) Name() string { return "github-desktop" }

// Check implements updater.Provider. It lists releases, keeps only the
// desktop-v tag namespace, picks the highest by semver, and returns it when it
// is newer than the running version. Returns (nil, nil) when up to date.
func (p *desktopProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	releases, err := p.listReleases(ctx)
	if err != nil {
		return nil, err
	}

	var best *apiRelease
	var bestVer string // canonical "vX.Y.Z"
	for i := range releases {
		rel := &releases[i]
		if rel.Draft || rel.Prerelease {
			continue
		}
		if !strings.HasPrefix(rel.TagName, p.prefix) {
			continue
		}
		ver := canonicalSemver(strings.TrimPrefix(rel.TagName, p.prefix))
		if !semver.IsValid(ver) {
			continue
		}
		if best == nil || semver.Compare(ver, bestVer) > 0 {
			best = rel
			bestVer = ver
		}
	}
	if best == nil {
		// No desktop release published yet — treat as up to date.
		return nil, nil
	}

	current := canonicalSemver(strings.TrimPrefix(strings.TrimPrefix(req.CurrentVersion, p.prefix), "v"))
	if !semver.IsValid(current) || semver.Compare(bestVer, current) <= 0 {
		return nil, nil
	}

	assetIdx := pickZipAsset(best.Assets)
	if assetIdx < 0 {
		return nil, fmt.Errorf("desktop updater: release %s has no .zip asset", best.TagName)
	}
	picked := best.Assets[assetIdx]

	out := &updater.Release{
		Version:     strings.TrimPrefix(bestVer, "v"),
		Channel:     "stable",
		Name:        best.Name,
		Notes:       best.Body,
		PublishedAt: best.PublishedAt,
		Artifact: updater.Artifact{
			Filename: picked.Name,
			Filetype: "zip",
			Size:     picked.Size,
			Platform: req.Platform,
			Arch:     req.Arch,
		},
		Metadata: map[string]any{
			"github.asset.url":   picked.BrowserDownloadURL,
			"github.release.tag": best.TagName,
			"github.release.url": best.HTMLURL,
		},
	}

	digest, err := p.fetchDigest(ctx, best.Assets, picked.Name)
	if err != nil {
		return nil, fmt.Errorf("desktop updater: load checksum sidecar: %w", err)
	}
	if digest != nil {
		out.Verification = &updater.Verification{DigestAlgo: "sha256", Digest: digest}
	}

	return out, nil
}

// Download implements updater.Provider by delegating to the stock github
// provider, which reads the asset URL we stashed in Metadata.
func (p *desktopProvider) Download(ctx context.Context, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	return p.download.Download(ctx, rel, dst, onProgress)
}

// listReleases fetches a page of releases (newest first) from the list API.
func (p *desktopProvider) listReleases(ctx context.Context) ([]apiRelease, error) {
	endpoint := p.base + "/repos/" + p.repo + "/releases?per_page=30"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	p.setAuth(req)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("desktop updater: api request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, nil
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("desktop updater: api %d: %s", resp.StatusCode, body)
	}
	var list []apiRelease
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("desktop updater: decode releases: %w", err)
	}
	return list, nil
}

// fetchDigest downloads the SHA256SUMS sidecar (if present) and returns the
// raw digest bytes for target's basename, or nil when there is no sidecar.
func (p *desktopProvider) fetchDigest(ctx context.Context, assets []apiAsset, target string) ([]byte, error) {
	idx := -1
	for i, a := range assets {
		if a.Name == checksumAssetName {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assets[idx].BrowserDownloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	p.setAuth(req)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("checksum sidecar HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return parseChecksum(string(body), target)
}

func (p *desktopProvider) setAuth(req *http.Request) {
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
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

// pickZipAsset returns the index of the single .zip release asset (excluding
// the checksum sidecar), or -1 when none is present.
func pickZipAsset(assets []apiAsset) int {
	for i, a := range assets {
		if a.Name == checksumAssetName {
			continue
		}
		if strings.HasSuffix(strings.ToLower(a.Name), ".zip") {
			return i
		}
	}
	return -1
}

// parseChecksum extracts the digest for target from a sha256sum-style listing.
// Each line is "<hex-digest>  <filename>"; comparison is on the basename and
// tolerates the "*" binary-mode marker.
func parseChecksum(body, target string) ([]byte, error) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[len(fields)-1]
		name = strings.TrimPrefix(name, "*")
		name = strings.TrimPrefix(name, "./")
		if name != target {
			continue
		}
		digest, err := hex.DecodeString(fields[0])
		if err != nil {
			return nil, fmt.Errorf("malformed digest for %s: %w", target, err)
		}
		return digest, nil
	}
	return nil, errors.New("no checksum entry for " + target)
}

// --- GitHub API shapes ---

type apiRelease struct {
	TagName     string     `json:"tag_name"`
	Name        string     `json:"name"`
	Body        string     `json:"body"`
	Prerelease  bool       `json:"prerelease"`
	Draft       bool       `json:"draft"`
	PublishedAt time.Time  `json:"published_at"`
	HTMLURL     string     `json:"html_url"`
	Assets      []apiAsset `json:"assets"`
}

type apiAsset struct {
	Name               string `json:"name"`
	ContentType        string `json:"content_type"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}
