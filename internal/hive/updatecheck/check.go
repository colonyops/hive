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
)

var releaseHTTPClient = &http.Client{Timeout: 5 * time.Second}

var fetchLatestReleaseJSON = func(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseAPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "hive-update-checker")

	resp, err := releaseHTTPClient.Do(req)
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

// Check compares currentVersion to the latest release and returns a non-nil
// Result only when an update is available.
func Check(ctx context.Context, kvStore kv.KV, currentVersion string) (*Result, error) {
	if kvStore == nil || currentVersion == "" || currentVersion == "dev" {
		return nil, nil
	}

	normalizedCurrent, ok := normalizeVersion(currentVersion)
	if !ok {
		log.Debug().Str("version", currentVersion).Msg("update check: invalid current version")
		return nil, nil
	}

	release, err := getLatestRelease(ctx, kvStore)
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

func getLatestRelease(ctx context.Context, kvStore kv.KV) (ReleaseInfo, error) {
	cache := kv.Scoped[ReleaseInfo](kvStore, cacheNamespace)

	if cached, err := cache.Get(ctx, cacheKey); err == nil {
		return cached, nil
	}

	info, err := fetchRelease(ctx)
	if err != nil {
		return ReleaseInfo{}, err
	}

	if err := cache.SetTTL(ctx, cacheKey, info, cacheTTL); err != nil {
		log.Debug().Err(err).Msg("update check: failed to cache release")
	}

	return info, nil
}

func fetchRelease(ctx context.Context) (ReleaseInfo, error) {
	output, err := fetchLatestReleaseJSON(ctx)
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
