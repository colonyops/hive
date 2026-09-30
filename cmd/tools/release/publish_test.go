package main

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParsePublishOptionsRequiresUploadSkipWithNotarySkip(t *testing.T) {
	t.Parallel()

	_, err := parsePublishOptions([]string{"1.2.3-dev.1", "--skip-notarize"})
	if err == nil || !strings.Contains(err.Error(), "requires --skip-upload") {
		t.Fatalf("parsePublishOptions() error = %v", err)
	}
}

func TestParsePublishOptionsResumeNeedsNoSigningSecrets(t *testing.T) {
	for _, name := range []string{"MACOS_CERTIFICATE", "MACOS_CERTIFICATE_PWD", "MACOS_SIGN_IDENTITY", "AC_API_KEY", "AC_API_KEY_ID", "AC_API_ISSUER_ID"} {
		t.Setenv(name, "")
	}
	t.Setenv("R2_ACCESS_KEY_ID", "test")
	t.Setenv("R2_SECRET_ACCESS_KEY", "test")

	options, err := parsePublishOptions([]string{"1.2.3-dev.1", "--resume"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.resume {
		t.Fatalf("unexpected options: %#v", options)
	}
}

func TestParsePublishOptionsRejectsUnsafeResumeCombinations(t *testing.T) {
	for _, flag := range []string{"--force", "--skip-upload", "--skip-notarize"} {
		t.Run(flag, func(t *testing.T) {
			_, err := parsePublishOptions([]string{"1.2.3-dev.1", "--resume", flag})
			if err == nil || !strings.Contains(err.Error(), "--resume cannot be combined") {
				t.Fatalf("parsePublishOptions() error = %v", err)
			}
		})
	}
}

func TestParsePublishOptionsResumeStillRequiresR2Credentials(t *testing.T) {
	t.Setenv("R2_ACCESS_KEY_ID", "")
	t.Setenv("R2_SECRET_ACCESS_KEY", "")

	_, err := parsePublishOptions([]string{"1.2.3-dev.1", "--resume"})
	if err == nil || !strings.Contains(err.Error(), "R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY") {
		t.Fatalf("parsePublishOptions() error = %v", err)
	}
}

func TestR2CurlRetriesAreBounded(t *testing.T) {
	t.Parallel()

	args := r2CurlRetryArgs()
	for _, value := range []string{"--retry", "5", "--retry-all-errors", "--connect-timeout", "30"} {
		if !slices.Contains(args, value) {
			t.Fatalf("r2CurlRetryArgs() = %q, missing %q", args, value)
		}
	}
	// curl starts the --retry-max-time timer before the first attempt, so a cap
	// silently disables retries for the artifact uploads that most need them.
	if slices.Contains(args, "--retry-max-time") {
		t.Fatalf("r2CurlRetryArgs() = %q, must not cap retry wall-clock", args)
	}
}

func TestChooseR2UploadAction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		exists, matches bool
		resume, force   bool
		want            r2UploadAction
		wantErr         bool
	}{
		{name: "missing normal object", want: r2UploadObject},
		{name: "existing normal object conflicts", exists: true, wantErr: true},
		{name: "matching resume object is reused", exists: true, matches: true, resume: true, want: r2ReuseObject},
		{name: "different resume object conflicts", exists: true, resume: true, wantErr: true},
		{name: "force overwrites existing object", exists: true, force: true, want: r2UploadObject},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got, err := chooseR2UploadAction(testCase.exists, testCase.matches, testCase.resume, testCase.force)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("chooseR2UploadAction() error = %v, wantErr %t", err, testCase.wantErr)
			}
			if err == nil && got != testCase.want {
				t.Fatalf("chooseR2UploadAction() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestParsePublishOptionsAllowsLocalUnnotarizedBuild(t *testing.T) {
	for _, name := range []string{"MACOS_CERTIFICATE", "MACOS_CERTIFICATE_PWD", "MACOS_SIGN_IDENTITY"} {
		t.Setenv(name, "test")
	}

	options, err := parsePublishOptions([]string{"1.2.3-dev.1", "--skip-notarize", "--skip-upload"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.skipNotarize || !options.skipUpload {
		t.Fatalf("unexpected options: %#v", options)
	}
}

// The two-space separator is what `sha256sum -c` requires; the install
// instructions in docs/distribution.md pipe SHA256SUMS straight into it.
func TestWriteChecksumsMatchesSha256sumFormat(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join(desktopDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	publisher := &publisher{}
	err := publisher.writeChecksums([]releaseArtifact{
		{checksum: strings.Repeat("a", 64), name: "Hive-1.2.3-darwin-universal.zip"},
		{checksum: strings.Repeat("b", 64), name: "Hive-1.2.3-darwin-universal.dmg"},
		{checksum: strings.Repeat("c", 64), name: "Hive-1.2.3-linux-amd64.tar.gz"},
	})
	if err != nil {
		t.Fatal(err)
	}

	contents, err := os.ReadFile(filepath.Join(desktopDir, "bin", "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("a", 64) + "  Hive-1.2.3-darwin-universal.zip\n" +
		strings.Repeat("b", 64) + "  Hive-1.2.3-darwin-universal.dmg\n" +
		strings.Repeat("c", 64) + "  Hive-1.2.3-linux-amd64.tar.gz\n"
	if string(contents) != want {
		t.Fatalf("SHA256SUMS = %q, want %q", contents, want)
	}
}

// The zip and the DMG share a platform key; they must land in different fields
// of one entry, because the updater reads url and only url.
func TestPlatformManifestsPairUpdateAndInstaller(t *testing.T) {
	t.Parallel()

	platforms, err := platformManifests("https://dl.example.com", "desktop/releases/1.2.3", []releaseArtifact{
		{platformKey: "darwin-universal", role: updateArtifact, name: "Hive-1.2.3-darwin-universal.zip", checksum: strings.Repeat("a", 64), size: 10},
		{platformKey: "darwin-universal", role: installerArtifact, name: "Hive-1.2.3-darwin-universal.dmg", checksum: strings.Repeat("b", 64), size: 20},
		{platformKey: "linux-amd64", role: updateArtifact, name: "Hive-1.2.3-linux-amd64.tar.gz", checksum: strings.Repeat("c", 64), size: 30},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]platformManifest{
		"darwin-universal": {
			URL:             "https://dl.example.com/desktop/releases/1.2.3/Hive-1.2.3-darwin-universal.zip",
			SHA256:          strings.Repeat("a", 64),
			Size:            10,
			InstallerURL:    "https://dl.example.com/desktop/releases/1.2.3/Hive-1.2.3-darwin-universal.dmg",
			InstallerSHA256: strings.Repeat("b", 64),
			InstallerSize:   20,
		},
		"linux-amd64": {
			URL:    "https://dl.example.com/desktop/releases/1.2.3/Hive-1.2.3-linux-amd64.tar.gz",
			SHA256: strings.Repeat("c", 64),
			Size:   30,
		},
	}
	if !maps.Equal(platforms, want) {
		t.Fatalf("platformManifests() = %#v, want %#v", platforms, want)
	}
}

func TestValidateResumeManifestsAllowsPartialCascade(t *testing.T) {
	t.Parallel()

	version := mustVersion(t, "1.2.3-beta.2")
	platforms := map[string]platformManifest{
		"darwin-universal": {URL: "https://example.com/app.zip", SHA256: strings.Repeat("a", 64), Size: 10},
	}
	manifests := map[string]channelManifest{
		"beta": {
			Channel: "beta", Version: version.String(), Summary: "Summary", Notes: "Notes", Platforms: maps.Clone(platforms),
		},
		"dev": {
			Channel: "dev", Version: "1.2.3-dev.8", Platforms: maps.Clone(platforms),
		},
	}
	if err := validateResumeManifests(version, manifests, "Summary", "Notes", platforms); err != nil {
		t.Fatal(err)
	}
}

func TestValidateResumeManifestsRejectsConflicts(t *testing.T) {
	t.Parallel()

	version := mustVersion(t, "1.2.3-dev.7")
	platforms := map[string]platformManifest{
		"darwin-universal": {URL: "https://example.com/app.zip", SHA256: strings.Repeat("a", 64), Size: 10},
	}
	cases := []struct {
		name     string
		manifest channelManifest
		want     string
	}{
		{
			name: "same version different metadata",
			manifest: channelManifest{
				Channel: "dev", Version: version.String(), Summary: "Different", Notes: "Notes", Platforms: maps.Clone(platforms),
			},
			want: "conflicting release metadata",
		},
		{
			name: "newer manifest",
			manifest: channelManifest{
				Channel: "dev", Version: "1.2.3-dev.8", Platforms: maps.Clone(platforms),
			},
			want: "older than",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := validateResumeManifests(version, map[string]channelManifest{"dev": testCase.manifest}, "Summary", "Notes", platforms)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("validateResumeManifests() error = %v, want %q", err, testCase.want)
			}
		})
	}
}

// An artifact with no role would otherwise publish a manifest entry missing
// either the update URL or the installer URL, silently.
func TestPlatformManifestsRejectsARolelessArtifact(t *testing.T) {
	t.Parallel()

	_, err := platformManifests("https://dl.example.com", "desktop/releases/1.2.3", []releaseArtifact{
		{platformKey: "darwin-universal", name: "Hive-1.2.3-darwin-universal.zip", checksum: strings.Repeat("a", 64), size: 10},
	})
	if err == nil {
		t.Fatal("platformManifests accepted an artifact with no role")
	}
}
