package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchManifest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing.json" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("User-Agent") != "hive-desktop-release/1" {
			http.Error(w, "unexpected user agent", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"channel":"dev","version":"0.1.8-dev.1","pub_date":"2026-07-24T00:00:00Z","platforms":{"darwin-universal":{"url":"https://example.com/Hive.zip","sha256":"0000000000000000000000000000000000000000000000000000000000000000","size":1}}}`))
	}))
	defer server.Close()

	manifest, err := fetchManifest(context.Background(), server.Client(), server.URL+"/latest.json")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Channel != "dev" || manifest.Version != "0.1.8-dev.1" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}

	_, err = fetchManifest(context.Background(), server.Client(), server.URL+"/missing.json")
	if !errors.Is(err, errManifestNotFound) {
		t.Fatalf("missing manifest error = %v", err)
	}
}

// Installer metadata is optional so manifests published before the DMG existed
// still parse, but a half-filled group is rejected: a consumer that read the URL
// and skipped the missing checksum would install unverified bytes.
func TestValidatePlatformManifestInstallerMetadata(t *testing.T) {
	t.Parallel()

	digest := strings.Repeat("a", 64)
	base := platformManifest{URL: "https://example.com/Hive.zip", SHA256: digest, Size: 1}

	tests := []struct {
		name     string
		mutate   func(*platformManifest)
		wantErr  bool
		platform platformManifest
	}{
		{name: "absent", mutate: func(*platformManifest) {}},
		{name: "complete", mutate: func(p *platformManifest) {
			p.InstallerURL, p.InstallerSHA256, p.InstallerSize = "https://example.com/Hive.dmg", digest, 2
		}},
		{name: "url without checksum", wantErr: true, mutate: func(p *platformManifest) {
			p.InstallerURL, p.InstallerSize = "https://example.com/Hive.dmg", 2
		}},
		{name: "checksum without url", wantErr: true, mutate: func(p *platformManifest) {
			p.InstallerSHA256, p.InstallerSize = digest, 2
		}},
		{name: "malformed checksum", wantErr: true, mutate: func(p *platformManifest) {
			p.InstallerURL, p.InstallerSHA256, p.InstallerSize = "https://example.com/Hive.dmg", "not-hex", 2
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			platform := base
			tt.mutate(&platform)
			err := validatePlatformManifest("darwin-universal", platform)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validatePlatformManifest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateManifestAdvancementChecksCascade(t *testing.T) {
	t.Parallel()

	candidate := mustParseVersion(t, "1.2.3")
	manifests := map[string]channelManifest{
		"stable": {Version: "1.2.2"},
		"beta":   {Version: "1.2.3-beta.1"},
		"dev":    {Version: "1.2.4-dev.1"},
	}
	if err := validateManifestAdvancement(candidate, manifests); err == nil {
		t.Fatal("validateManifestAdvancement unexpectedly allowed a dev channel downgrade")
	}

	candidate = mustParseVersion(t, "1.2.4")
	if err := validateManifestAdvancement(candidate, manifests); err != nil {
		t.Fatalf("validateManifestAdvancement rejected advancing stable release: %v", err)
	}
}

func TestFetchManifestRejectsMissingFields(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"channel":"dev"}`))
	}))
	defer server.Close()

	if _, err := fetchManifest(context.Background(), server.Client(), server.URL); err == nil {
		t.Fatal("fetchManifest unexpectedly accepted an incomplete manifest")
	}
}

// Manifests published before Linux shipped carry only darwin-universal and must
// still parse, since advancement checks read the live channels.
func TestValidateManifestAcceptsLegacyAndMultiPlatform(t *testing.T) {
	t.Parallel()

	const digest = "0000000000000000000000000000000000000000000000000000000000000000"
	base := func(platforms map[string]platformManifest) channelManifest {
		return channelManifest{
			Channel:   "dev",
			Version:   "0.1.8-dev.1",
			PubDate:   "2026-07-24T00:00:00Z",
			Platforms: platforms,
		}
	}
	good := platformManifest{URL: "https://example.com/a", SHA256: digest, Size: 1}

	if err := validateManifest(base(map[string]platformManifest{"darwin-universal": good})); err != nil {
		t.Fatalf("darwin-only manifest rejected: %v", err)
	}
	if err := validateManifest(base(map[string]platformManifest{
		"darwin-universal": good,
		"linux-amd64":      good,
		"linux-arm64":      good,
	})); err != nil {
		t.Fatalf("multi-platform manifest rejected: %v", err)
	}

	// A malformed non-darwin entry must fail too — it is what the updater hands
	// to a Linux client.
	err := validateManifest(base(map[string]platformManifest{
		"darwin-universal": good,
		"linux-amd64":      {URL: "https://example.com/a", SHA256: "not-hex", Size: 1},
	}))
	if err == nil {
		t.Fatal("expected a malformed linux-amd64 entry to be rejected")
	}
	if !strings.Contains(err.Error(), "linux-amd64") {
		t.Fatalf("error %q does not name the offending platform", err)
	}

	err = validateManifest(base(map[string]platformManifest{"linux-amd64": good}))
	if err == nil {
		t.Fatal("expected a manifest without darwin-universal to be rejected")
	}
	if !strings.Contains(err.Error(), "darwin-universal") {
		t.Fatalf("error %q does not name the missing platform", err)
	}
}

// Every affected channel of one release must advertise the same platform set;
// a channel missing a platform would break updates for those clients only.
func TestVerifyLiveRejectsDivergentChannelManifests(t *testing.T) {
	const digest = "0000000000000000000000000000000000000000000000000000000000000000"
	manifest := func(channel string, platforms map[string]platformManifest) channelManifest {
		return channelManifest{
			Channel:   channel,
			Version:   "1.2.3-beta.1",
			PubDate:   "2026-07-24T00:00:00Z",
			Platforms: platforms,
		}
	}
	good := platformManifest{URL: "https://example.com/a", SHA256: digest, Size: 1}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/beta/latest.json":
			_ = json.NewEncoder(w).Encode(manifest("beta", map[string]platformManifest{
				"darwin-universal": good,
				"linux-amd64":      good,
				"linux-arm64":      good,
			}))
		case "/dev/latest.json":
			_ = json.NewEncoder(w).Encode(manifest("dev", map[string]platformManifest{
				"darwin-universal": good,
				"linux-amd64":      good,
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("HIVE_DESKTOP_MANIFEST_BASE", server.URL)

	version, err := parseVersion("1.2.3-beta.1")
	if err != nil {
		t.Fatal(err)
	}
	err = verifyLive(context.Background(), version)
	if err == nil {
		t.Fatal("expected divergent channel manifests to be rejected")
	}
	if !strings.Contains(err.Error(), "differ") {
		t.Fatalf("error %q does not report the divergence", err)
	}
}
