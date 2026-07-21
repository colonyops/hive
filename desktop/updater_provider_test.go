package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

// fixtureServer serves a releases list plus a SHA256SUMS body, mirroring the
// GitHub API shape the provider consumes. The zip's browser_download_url and
// the SHA256SUMS asset both point back at this server.
type fixtureServer struct {
	*httptest.Server
	zipBody      []byte
	checksumBody string
}

func newFixtureServer(t *testing.T, releasesJSON func(base string) string, checksumBody string) *fixtureServer {
	t.Helper()
	fs := &fixtureServer{zipBody: []byte("PK\x03\x04 fake zip"), checksumBody: checksumBody}
	mux := http.NewServeMux()
	fs.Server = httptest.NewServer(mux)
	base := fs.Server.URL
	mux.HandleFunc("/repos/colonyops/hive/releases", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, releasesJSON(base))
	})
	mux.HandleFunc("/dl/SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, fs.checksumBody)
	})
	mux.HandleFunc("/dl/zip", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fs.zipBody)
	})
	t.Cleanup(fs.Server.Close)
	return fs
}

func newTestProvider(t *testing.T, base string) *desktopProvider {
	t.Helper()
	p, err := newDesktopProvider("colonyops/hive", "")
	require.NoError(t, err)
	p.base = base
	return p
}

// releasesWithMixedTags returns a list containing a newer CLI v* release, an
// older desktop release, and the newest desktop release, in newest-first
// order — so the provider must filter by prefix and compare semver rather than
// trust list order.
func releasesWithMixedTags(zipName, sumsName string) func(base string) string {
	return func(base string) string {
		return fmt.Sprintf(`[
  {"tag_name":"v9.9.9","name":"CLI","draft":false,"prerelease":false,"assets":[]},
  {"tag_name":"desktop-v0.3.0","name":"Desktop 0.3.0","body":"notes","draft":false,"prerelease":false,"html_url":"https://example/desktop-v0.3.0","assets":[
    {"name":%q,"size":9,"browser_download_url":%q},
    {"name":%q,"size":80,"browser_download_url":%q}
  ]},
  {"tag_name":"desktop-v0.2.0","name":"Desktop 0.2.0","draft":false,"prerelease":false,"assets":[]},
  {"tag_name":"desktop-v0.4.0-rc.1","name":"RC","draft":false,"prerelease":true,"assets":[]}
]`, zipName, base+"/dl/zip", sumsName, base+"/dl/SHA256SUMS")
	}
}

func checksumFor(body []byte, name string) string {
	sum := sha256.Sum256(body)
	return fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), name)
}

func TestDesktopProviderCheckNewer(t *testing.T) {
	zipName := "Hive-desktop-0.3.0-macos-universal.zip"
	fs := newFixtureServer(t, releasesWithMixedTags(zipName, checksumAssetName), "")
	fs.checksumBody = checksumFor(fs.zipBody, zipName)
	p := newTestProvider(t, fs.Server.URL)

	rel, err := p.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.2.0"})
	require.NoError(t, err)
	require.NotNil(t, rel)
	require.Equal(t, "0.3.0", rel.Version)
	require.Equal(t, zipName, rel.Artifact.Filename)
	require.Equal(t, fs.Server.URL+"/dl/zip", rel.Metadata["github.asset.url"])
	require.NotNil(t, rel.Verification)
	require.Equal(t, "sha256", rel.Verification.DigestAlgo)
	want := sha256.Sum256(fs.zipBody)
	require.Equal(t, want[:], rel.Verification.Digest)
}

func TestDesktopProviderCheckUpToDate(t *testing.T) {
	zipName := "Hive-desktop-0.3.0-macos-universal.zip"
	fs := newFixtureServer(t, releasesWithMixedTags(zipName, checksumAssetName), checksumFor([]byte("PK\x03\x04 fake zip"), zipName))
	p := newTestProvider(t, fs.Server.URL)

	// Current equals the newest desktop release.
	rel, err := p.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.3.0"})
	require.NoError(t, err)
	require.Nil(t, rel)

	// Current newer than any published desktop release.
	rel, err = p.Check(context.Background(), updater.CheckRequest{CurrentVersion: "1.0.0"})
	require.NoError(t, err)
	require.Nil(t, rel)
}

func TestDesktopProviderCheckAcceptsDesktopPrefixedCurrent(t *testing.T) {
	zipName := "Hive-desktop-0.3.0-macos-universal.zip"
	fs := newFixtureServer(t, releasesWithMixedTags(zipName, checksumAssetName), checksumFor([]byte("PK\x03\x04 fake zip"), zipName))
	p := newTestProvider(t, fs.Server.URL)

	rel, err := p.Check(context.Background(), updater.CheckRequest{CurrentVersion: "desktop-v0.2.0"})
	require.NoError(t, err)
	require.NotNil(t, rel)
	require.Equal(t, "0.3.0", rel.Version)
}

func TestDesktopProviderCheckNoDesktopReleases(t *testing.T) {
	onlyCLI := func(string) string {
		return `[{"tag_name":"v9.9.9","draft":false,"prerelease":false,"assets":[]}]`
	}
	fs := newFixtureServer(t, onlyCLI, "")
	p := newTestProvider(t, fs.Server.URL)

	rel, err := p.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.1.0"})
	require.NoError(t, err)
	require.Nil(t, rel)
}

func TestDesktopProviderCheckMissingZip(t *testing.T) {
	noZip := func(base string) string {
		return fmt.Sprintf(`[{"tag_name":"desktop-v0.3.0","draft":false,"prerelease":false,"assets":[
      {"name":%q,"browser_download_url":%q}
    ]}]`, checksumAssetName, base+"/dl/SHA256SUMS")
	}
	fs := newFixtureServer(t, noZip, "whatever")
	p := newTestProvider(t, fs.Server.URL)

	_, err := p.Check(context.Background(), updater.CheckRequest{CurrentVersion: "0.1.0"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no .zip asset")
}

func TestParseChecksum(t *testing.T) {
	body := "abc  other.zip\n" +
		"0011aa  Hive-desktop-0.3.0-macos-universal.zip\n"
	digest, err := parseChecksum(body, "Hive-desktop-0.3.0-macos-universal.zip")
	require.NoError(t, err)
	require.Equal(t, []byte{0x00, 0x11, 0xaa}, digest)

	_, err = parseChecksum(body, "missing.zip")
	require.Error(t, err)
}

func TestPickZipAsset(t *testing.T) {
	assets := []apiAsset{
		{Name: checksumAssetName},
		{Name: "Hive-desktop-0.3.0-macos-universal.zip"},
	}
	require.Equal(t, 1, pickZipAsset(assets))
	require.Equal(t, -1, pickZipAsset([]apiAsset{{Name: "notes.txt"}}))
}
