package wailsui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/updater"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// manifestServer serves a channel latest.json plus the artifact zip it points
// at, mirroring the bucket layout the release CLI publishes.
type manifestServer struct {
	*httptest.Server
	zipBody []byte
}

// newManifestServer serves manifestJSON (a func of the server base URL so the
// artifact URL can point back at the server) for channel and the zip at
// /desktop/releases/<version>/<zipName>.
func newManifestServer(t *testing.T, channel string, manifestJSON func(base string) string) *manifestServer {
	t.Helper()
	ms := &manifestServer{zipBody: []byte("PK\x03\x04 fake zip")}
	mux := http.NewServeMux()
	ms.Server = httptest.NewServer(mux)
	base := ms.URL
	mux.HandleFunc("/desktop/channels/"+channel+"/latest.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, manifestJSON(base))
	})
	mux.HandleFunc("/desktop/releases/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(ms.zipBody)
	})
	t.Cleanup(ms.Close)
	return ms
}

// stableManifest returns a well-formed stable-channel manifest for version
// whose artifact URL and sha256 match the server's zip body.
func stableManifest(zipBody []byte, version string) func(base string) string {
	sum := sha256.Sum256(zipBody)
	return func(base string) string {
		return fmt.Sprintf(`{
  "channel": "stable",
  "version": %q,
  "pub_date": "2026-08-01T00:00:00Z",
  "platforms": {
    "darwin-universal": {
      "url": "%s/desktop/releases/%s/Hive-%s-darwin-universal.zip",
      "sha256": %q,
      "size": %d
    }
  }
}`, version, base, version, version, hex.EncodeToString(sum[:]), len(zipBody))
	}
}

func darwinCheck(current string) updater.CheckRequest {
	return updater.CheckRequest{CurrentVersion: current, Platform: "darwin", Arch: "arm64"}
}

func TestManifestProviderCheckNewer(t *testing.T) {
	ms := newManifestServer(t, settings.ChannelStable, stableManifest([]byte("PK\x03\x04 fake zip"), "1.4.0"))
	p := NewManifestProvider(ms.URL, settings.ChannelStable)

	rel, err := p.Check(context.Background(), darwinCheck("1.3.0"))
	require.NoError(t, err)
	require.NotNil(t, rel)
	require.Equal(t, "1.4.0", rel.Version)
	require.Equal(t, settings.ChannelStable, rel.Channel)
	require.Equal(t, "Hive-1.4.0-darwin-universal.zip", rel.Artifact.Filename)
	require.Equal(t, int64(len(ms.zipBody)), rel.Artifact.Size)
	require.Equal(t, ms.URL+"/desktop/releases/1.4.0/Hive-1.4.0-darwin-universal.zip", rel.Metadata[artifactURLKey])
	require.NotNil(t, rel.Verification)
	require.Equal(t, "sha256", rel.Verification.DigestAlgo)
	want := sha256.Sum256(ms.zipBody)
	require.Equal(t, want[:], rel.Verification.Digest)
}

// Releases publish a .dmg installer beside the update zip and advertise it in
// the same platform entry (decision 0060). The updater has no disk-image path,
// so it must keep resolving `url` and ignore the installer fields — including
// in builds already shipped, which decode this manifest with the same tolerance.
func TestManifestProviderCheckIgnoresInstallerFields(t *testing.T) {
	zipBody := []byte("PK\x03\x04 fake zip")
	sum := sha256.Sum256(zipBody)
	ms := newManifestServer(t, settings.ChannelStable, func(base string) string {
		return fmt.Sprintf(`{
  "channel": "stable",
  "version": "1.4.0",
  "pub_date": "2026-08-01T00:00:00Z",
  "platforms": {
    "darwin-universal": {
      "url": "%s/desktop/releases/1.4.0/Hive-1.4.0-darwin-universal.zip",
      "sha256": %q,
      "size": %d,
      "installer_url": "%s/desktop/releases/1.4.0/Hive-1.4.0-darwin-universal.dmg",
      "installer_sha256": "%s",
      "installer_size": 99
    }
  }
}`, base, hex.EncodeToString(sum[:]), len(zipBody), base, hex.EncodeToString(bytes.Repeat([]byte{0xab}, sha256.Size)))
	})
	p := NewManifestProvider(ms.URL, settings.ChannelStable)

	rel, err := p.Check(context.Background(), darwinCheck("1.3.0"))
	require.NoError(t, err)
	require.NotNil(t, rel)
	require.Equal(t, "Hive-1.4.0-darwin-universal.zip", rel.Artifact.Filename)
	require.Equal(t, "zip", rel.Artifact.Filetype)
	require.Equal(t, int64(len(zipBody)), rel.Artifact.Size)
	require.Equal(t, ms.URL+"/desktop/releases/1.4.0/Hive-1.4.0-darwin-universal.zip", rel.Metadata[artifactURLKey])
	require.Equal(t, sum[:], rel.Verification.Digest)
}

func TestManifestProviderCheckUpToDate(t *testing.T) {
	ms := newManifestServer(t, settings.ChannelStable, stableManifest([]byte("PK\x03\x04 fake zip"), "1.4.0"))
	p := NewManifestProvider(ms.URL, settings.ChannelStable)

	// Current equals the manifest version.
	rel, err := p.Check(context.Background(), darwinCheck("1.4.0"))
	require.NoError(t, err)
	require.Nil(t, rel)

	// Current newer than the manifest version.
	rel, err = p.Check(context.Background(), darwinCheck("2.0.0"))
	require.NoError(t, err)
	require.Nil(t, rel)
}

func TestManifestProviderCheckAcceptsPrefixedCurrent(t *testing.T) {
	ms := newManifestServer(t, settings.ChannelStable, stableManifest([]byte("PK\x03\x04 fake zip"), "1.4.0"))
	p := NewManifestProvider(ms.URL, settings.ChannelStable)

	rel, err := p.Check(context.Background(), darwinCheck("desktop-v1.3.0"))
	require.NoError(t, err)
	require.NotNil(t, rel)
	require.Equal(t, "1.4.0", rel.Version)
}

// TestManifestProviderPrereleaseOrdering exercises the ordering of the
// prerelease history (ADR release-channels), which installs from before the
// shared version still follow on the dev channel: a newer dev build updates a
// dev user, a beta manifest never downgrades a dev user of the same base
// version, and a bare version converges everyone.
func TestManifestProviderPrereleaseOrdering(t *testing.T) {
	devManifest := func(version string) func(base string) string {
		body := []byte("PK\x03\x04 fake zip")
		sum := sha256.Sum256(body)
		return func(base string) string {
			return fmt.Sprintf(`{"channel":"dev","version":%q,"pub_date":"2026-08-01T00:00:00Z",
  "platforms":{"darwin-universal":{"url":"%s/desktop/releases/%s/Hive-%s-darwin-universal.zip","sha256":%q,"size":%d}}}`,
				version, base, version, version, hex.EncodeToString(sum[:]), len(body))
		}
	}

	tests := []struct {
		name     string
		manifest string
		current  string
		wantsRel bool
	}{
		{"newer dev build", "1.4.0-dev.2", "1.4.0-dev.1", true},
		{"cascaded beta does not downgrade dev", "1.4.0-beta.2", "1.4.0-dev.1", false},
		{"stable converges dev users", "1.4.0", "1.4.0-dev.3", true},
		{"same dev build", "1.4.0-dev.1", "1.4.0-dev.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := newManifestServer(t, settings.ChannelDev, devManifest(tt.manifest))
			p := NewManifestProvider(ms.URL, settings.ChannelDev)
			rel, err := p.Check(context.Background(), darwinCheck(tt.current))
			require.NoError(t, err)
			if tt.wantsRel {
				require.NotNil(t, rel)
				require.Equal(t, tt.manifest, rel.Version)
			} else {
				require.Nil(t, rel)
			}
		})
	}
}

func TestManifestProviderChannelMismatch(t *testing.T) {
	// A beta manifest served from the stable channel path must be rejected.
	betaOnStablePath := func(string) string {
		return `{"channel":"beta","version":"1.4.0","platforms":{}}`
	}
	ms := newManifestServer(t, settings.ChannelStable, betaOnStablePath)
	p := NewManifestProvider(ms.URL, settings.ChannelStable)

	_, err := p.Check(context.Background(), darwinCheck("1.3.0"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match configured channel")
}

func TestManifestProviderCheckNoManifest(t *testing.T) {
	// No channel manifest published yet: the server 404s and the provider
	// reports up to date.
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	p := NewManifestProvider(srv.URL, settings.ChannelStable)

	rel, err := p.Check(context.Background(), darwinCheck("1.3.0"))
	require.NoError(t, err)
	require.Nil(t, rel)
}

func TestManifestProviderCheckMissingPlatform(t *testing.T) {
	noDarwin := func(string) string {
		return `{"channel":"stable","version":"1.4.0","platforms":{"linux-amd64":{"url":"https://example/zip","sha256":"00","size":1}}}`
	}
	ms := newManifestServer(t, settings.ChannelStable, noDarwin)
	p := NewManifestProvider(ms.URL, settings.ChannelStable)

	_, err := p.Check(context.Background(), darwinCheck("1.3.0"))
	require.Error(t, err)
	require.Contains(t, err.Error(), `no artifact for platform "darwin-universal"`)
}

func TestManifestProviderCheckMalformedSHA(t *testing.T) {
	badSHA := func(base string) string {
		return fmt.Sprintf(`{"channel":"stable","version":"1.4.0",
  "platforms":{"darwin-universal":{"url":"%s/desktop/releases/1.4.0/z.zip","sha256":"not-hex","size":1}}}`, base)
	}
	ms := newManifestServer(t, settings.ChannelStable, badSHA)
	p := NewManifestProvider(ms.URL, settings.ChannelStable)

	_, err := p.Check(context.Background(), darwinCheck("1.3.0"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "sha256")
}

func TestManifestProviderCheckMalformedVersion(t *testing.T) {
	badVersion := func(string) string {
		return `{"channel":"stable","version":"not-a-version","platforms":{}}`
	}
	ms := newManifestServer(t, settings.ChannelStable, badVersion)
	p := NewManifestProvider(ms.URL, settings.ChannelStable)

	_, err := p.Check(context.Background(), darwinCheck("1.3.0"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not valid semver")
}

func TestManifestProviderDownload(t *testing.T) {
	ms := newManifestServer(t, settings.ChannelStable, stableManifest([]byte("PK\x03\x04 fake zip"), "1.4.0"))
	p := NewManifestProvider(ms.URL, settings.ChannelStable)

	rel, err := p.Check(context.Background(), darwinCheck("1.3.0"))
	require.NoError(t, err)
	require.NotNil(t, rel)

	var dst bytes.Buffer
	var lastWritten, lastTotal int64
	err = p.Download(context.Background(), rel, &dst, func(written, total int64) {
		lastWritten, lastTotal = written, total
	})
	require.NoError(t, err)
	require.Equal(t, ms.zipBody, dst.Bytes())
	require.Equal(t, int64(len(ms.zipBody)), lastWritten)
	require.Equal(t, int64(len(ms.zipBody)), lastTotal)
}

func TestManifestProviderDownloadMissingMetadata(t *testing.T) {
	p := NewManifestProvider("https://example.invalid", settings.ChannelStable)
	err := p.Download(context.Background(), &updater.Release{}, &bytes.Buffer{}, nil)
	require.Error(t, err)
}

func TestPlatformKey(t *testing.T) {
	require.Equal(t, "darwin-universal", platformKey("darwin", "arm64"))
	require.Equal(t, "darwin-universal", platformKey("darwin", "amd64"))
	require.Equal(t, "linux-amd64", platformKey("linux", "amd64"))
	require.Equal(t, "windows-arm64", platformKey("windows", "arm64"))
}
