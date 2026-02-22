package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/colonyops/hive/internal/core/kv"
	"github.com/rs/zerolog/log"
	"golang.org/x/mod/semver"
)

const (
	cacheTTL       = 24 * time.Hour
	cacheNamespace = "update-check"
	cacheKey       = "latest"
	releaseAPIURL  = "https://api.github.com/repos/colonyops/hive/releases/latest"
	defaultTimeout = 5 * time.Second
)

// ReleaseInfo holds cached release data returned by GitHub.
type ReleaseInfo struct {
	TagName     string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
}

// Result is returned when a newer version is available.
type Result struct {
	Current string
	Latest  string
}

// Checker resolves latest releases and compares them against the current build.
type Checker struct {
	cache                  *kv.TypedKV[ReleaseInfo]
	client                 *http.Client
	releaseAPIURL          string
	fetchLatestReleaseJSON func(context.Context) ([]byte, error)
}

// New creates a new update checker bound to a KV store.
func New(kvStore kv.KV, client *http.Client) *Checker {
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}

	c := &Checker{
		client:        client,
		releaseAPIURL: releaseAPIURL,
	}
	if kvStore != nil {
		c.cache = kv.Scoped[ReleaseInfo](kvStore, cacheNamespace)
	}
	c.fetchLatestReleaseJSON = c.defaultFetchLatestReleaseJSON

	return c
}

// Check compares currentVersion to the latest release and returns a non-nil
// Result only when an update is available.
func (c *Checker) Check(ctx context.Context, currentVersion string) (*Result, error) {
	if c == nil || c.cache == nil || currentVersion == "" || currentVersion == "dev" {
		return nil, nil
	}

	normalizedCurrent, ok := normalizeVersion(currentVersion)
	if !ok {
		log.Debug().Str("version", currentVersion).Msg("update check: invalid current version")
		return nil, nil
	}

	release, err := c.getLatestRelease(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("update check: failed to get latest release")
		return nil, nil
	}

	normalizedLatest, ok := normalizeVersion(release.TagName)
	if !ok {
		log.Debug().Str("tag", release.TagName).Msg("update check: invalid release tag")
		return nil, nil
	}

	if semver.Compare(normalizedCurrent, normalizedLatest) >= 0 {
		return nil, nil
	}

	return &Result{Current: normalizedCurrent, Latest: normalizedLatest}, nil
}

func (c *Checker) getLatestRelease(ctx context.Context) (ReleaseInfo, error) {
	if cached, err := c.cache.Get(ctx, cacheKey); err == nil {
		return cached, nil
	}

	info, err := c.fetchRelease(ctx)
	if err != nil {
		return ReleaseInfo{}, err
	}

	if err := c.cache.SetTTL(ctx, cacheKey, info, cacheTTL); err != nil {
		log.Debug().Err(err).Msg("update check: failed to cache release")
	}

	return info, nil
}

func (c *Checker) fetchRelease(ctx context.Context) (ReleaseInfo, error) {
	output, err := c.fetchLatestReleaseJSON(ctx)
	if err != nil {
		return ReleaseInfo{}, fmt.Errorf("fetch latest release: %w", err)
	}

	var info ReleaseInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return ReleaseInfo{}, fmt.Errorf("decode latest release: %w", err)
	}

	if info.TagName == "" {
		return ReleaseInfo{}, fmt.Errorf("decode latest release: missing tag_name")
	}

	return info, nil
}

func (c *Checker) defaultFetchLatestReleaseJSON(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.releaseAPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "hive-update-checker")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request latest release: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Debug().Err(err).Msg("update check: close latest release response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request latest release: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read latest release body: %w", err)
	}

	return body, nil
}

func normalizeVersion(version string) (string, bool) {
	if semver.IsValid(version) {
		return version, true
	}

	withPrefix := "v" + version
	if semver.IsValid(withPrefix) {
		return withPrefix, true
	}

	return "", false
}
