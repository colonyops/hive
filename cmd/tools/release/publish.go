package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	defaultR2Bucket    = "hive-desktop-releases"
	defaultR2AccountID = "bce6b95e4e84d92b1972d3b55b6cfaf6"
)

// desktopDir is the desktop program's directory, relative to the repo root
// every command here runs from.
var desktopDir = filepath.Join("cmd", "desktop")

type publishOptions struct {
	version         releaseVersion
	skipNotarize    bool
	skipUpload      bool
	force           bool
	resume          bool
	r2Bucket        string
	r2AccountID     string
	r2AccessKey     string
	r2SecretKey     string
	downloadBase    string
	signCertificate string
	signPassword    string
	signIdentity    string
	notaryKey       string
	notaryKeyID     string
	notaryIssuerID  string
}

type publisher struct {
	options           publishOptions
	workDir           string
	keychainPath      string
	notaryKeyPath     string
	originalKeychains []string
	// Stamped into every platform's binary, so all artifacts in a release
	// report the same provenance.
	commit    string
	buildDate string
}

// artifactRole separates the file the in-app updater downloads from the one a
// human downloads to install. Both live in the same release prefix and the same
// manifest platform entry; only macOS publishes a distinct installer.
type artifactRole string

const (
	updateArtifact    artifactRole = "update"
	installerArtifact artifactRole = "installer"
)

// releaseArtifact is one published file and the manifest metadata describing it.
// A release produces one per platform key and role, and registers them together.
type releaseArtifact struct {
	platformKey string // manifest platforms key, e.g. "linux-amd64"
	role        artifactRole
	name        string // file name within the release prefix
	path        string // local path
	checksum    string // hex sha256
	size        int64
}

type releaseObject struct {
	key          string
	path         string
	contentType  string
	cacheControl string
}

type r2UploadAction int

const (
	r2UploadObject r2UploadAction = iota
	r2ReuseObject
)

func publish(ctx context.Context, args []string) error {
	options, err := parsePublishOptions(args)
	if err != nil {
		return err
	}
	// The changelog gate runs before anything is built: the notes are embedded
	// in the binary, so an entry written after the build would describe a
	// release that cannot display it.
	if err := validateChangelogEntry(options.version); err != nil {
		return err
	}
	if !options.skipUpload {
		if err := validatePublishSource(ctx, options.version); err != nil {
			return err
		}
		if !options.resume {
			manifests, err := readManifests(ctx)
			if err != nil {
				return err
			}
			if err := validateManifestAdvancement(options.version, manifests); err != nil {
				return err
			}
		}
	}
	p := &publisher{options: options}
	if err := p.run(ctx); err != nil {
		return err
	}
	if options.skipUpload {
		return nil
	}
	if err := verifyLive(ctx, options.version); err != nil {
		return err
	}
	// Source-side record, after the artifacts are live and verified. It runs last
	// because a failure here (e.g. gh outage) leaves a fully published release that
	// `release github <version>` re-records without re-uploading.
	return publishGitHubRelease(ctx, options.version)
}

func parsePublishOptions(args []string) (publishOptions, error) {
	var versionText string
	options := publishOptions{
		r2Bucket:     envDefault("R2_BUCKET", defaultR2Bucket),
		r2AccountID:  envDefault("R2_ACCOUNT_ID", defaultR2AccountID),
		downloadBase: downloadBaseURL(),
	}
	for _, arg := range args {
		switch arg {
		case "--skip-notarize":
			options.skipNotarize = true
		case "--skip-upload":
			options.skipUpload = true
		case "--force":
			options.force = true
		case "--resume":
			options.resume = true
		default:
			if strings.HasPrefix(arg, "-") {
				return publishOptions{}, fmt.Errorf("unknown flag %s", arg)
			}
			if versionText != "" {
				return publishOptions{}, errors.New("usage: release publish <version> [--skip-notarize] [--skip-upload] [--force] [--resume]")
			}
			versionText = arg
		}
	}
	if versionText == "" {
		return publishOptions{}, errors.New("usage: release publish <version> [--skip-notarize] [--skip-upload] [--force] [--resume]")
	}
	if options.resume && (options.skipNotarize || options.skipUpload || options.force) {
		return publishOptions{}, errors.New("--resume cannot be combined with --skip-notarize, --skip-upload, or --force")
	}
	if options.skipNotarize && !options.skipUpload {
		return publishOptions{}, errors.New("--skip-notarize requires --skip-upload; public releases must be notarized")
	}
	version, err := parsePublishVersion(versionText)
	if err != nil {
		return publishOptions{}, fmt.Errorf("invalid version %q: %w", versionText, err)
	}
	options.version = version
	options.signCertificate = os.Getenv("MACOS_CERTIFICATE")
	options.signPassword = os.Getenv("MACOS_CERTIFICATE_PWD")
	options.signIdentity = os.Getenv("MACOS_SIGN_IDENTITY")
	options.notaryKey = os.Getenv("AC_API_KEY")
	options.notaryKeyID = os.Getenv("AC_API_KEY_ID")
	options.notaryIssuerID = os.Getenv("AC_API_ISSUER_ID")
	options.r2AccessKey = os.Getenv("R2_ACCESS_KEY_ID")
	options.r2SecretKey = os.Getenv("R2_SECRET_ACCESS_KEY")

	var missing []string
	if !options.resume {
		missing = missingValues(map[string]string{
			"MACOS_CERTIFICATE":     options.signCertificate,
			"MACOS_CERTIFICATE_PWD": options.signPassword,
			"MACOS_SIGN_IDENTITY":   options.signIdentity,
		})
	}
	if !options.skipNotarize && !options.resume {
		missing = append(missing, missingValues(map[string]string{
			"AC_API_KEY":       options.notaryKey,
			"AC_API_KEY_ID":    options.notaryKeyID,
			"AC_API_ISSUER_ID": options.notaryIssuerID,
		})...)
	}
	if !options.skipUpload {
		missing = append(missing, missingValues(map[string]string{
			"R2_ACCESS_KEY_ID":     options.r2AccessKey,
			"R2_SECRET_ACCESS_KEY": options.r2SecretKey,
		})...)
	}
	if len(missing) > 0 {
		return publishOptions{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return options, nil
}

func missingValues(values map[string]string) []string {
	var missing []string
	for name, value := range values {
		if value == "" {
			missing = append(missing, name)
		}
	}
	slices.Sort(missing)
	return missing
}

func envDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func (p *publisher) run(ctx context.Context) error {
	if err := p.preflight(ctx); err != nil {
		return err
	}
	workDir, err := os.MkdirTemp("", "hive-desktop-release.*")
	if err != nil {
		return fmt.Errorf("create release work directory: %w", err)
	}
	p.workDir = workDir
	defer func() {
		if err := p.cleanup(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: release cleanup failed: %v\n", err)
		}
	}()

	commit, err := gitHead(ctx)
	if err != nil {
		return err
	}
	p.commit = commit
	p.buildDate = time.Now().UTC().Format(time.RFC3339)

	if p.options.resume {
		return p.resumePublish(ctx)
	}

	fmt.Printf("==> releasing %s (manifests: %s)\n", p.options.version, strings.Join(manifestChannels, " "))
	if err := p.build(ctx); err != nil {
		return err
	}
	if err := p.sign(ctx); err != nil {
		return err
	}
	if !p.options.skipNotarize {
		if err := p.notarize(ctx); err != nil {
			return err
		}
	} else {
		fmt.Println("==> skipping notarization (--skip-notarize)")
	}
	macArtifact, err := p.packageApp(ctx)
	if err != nil {
		return err
	}
	installer, err := p.packageInstaller(ctx)
	if err != nil {
		return err
	}
	artifacts := []releaseArtifact{macArtifact, installer}

	// Every platform ships in this one publish; a later top-up publish would
	// fail the manifest-advancement rule (decision 0028).
	for _, arch := range linuxArches {
		artifact, err := p.buildLinux(ctx, arch)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, artifact)
	}

	if err := p.writeChecksums(artifacts); err != nil {
		return err
	}
	if p.options.skipUpload {
		fmt.Println("==> skipping upload (--skip-upload); artifacts:")
		for _, artifact := range artifacts {
			fmt.Printf("    %s\n", artifact.path)
		}
		return nil
	}
	// Re-check source state and manifests immediately before the irreversible
	// upload. The build and Apple notarization can take hours, so either may have
	// changed since the fail-fast validation at command startup.
	if err := validatePublishSource(ctx, p.options.version); err != nil {
		return err
	}
	manifests, err := readManifests(ctx)
	if err != nil {
		return err
	}
	if err := validateManifestAdvancement(p.options.version, manifests); err != nil {
		return err
	}
	return p.upload(ctx, artifacts, nil)
}

// writeChecksums writes one SHA256SUMS covering every artifact in the release.
// A single release process writes it once, so it stays consistent with the
// immutable prefix it lives in.
func (p *publisher) writeChecksums(artifacts []releaseArtifact) error {
	var contents strings.Builder
	for _, artifact := range artifacts {
		fmt.Fprintf(&contents, "%s  %s\n", artifact.checksum, artifact.name)
	}
	return os.WriteFile(filepath.Join(desktopDir, "bin", "SHA256SUMS"), []byte(contents.String()), 0o644)
}

func (p *publisher) preflight(ctx context.Context) error {
	if err := verifyMigrationOrder(ctx); err != nil {
		return err
	}

	var tools []string
	if p.options.resume {
		tools = []string{"codesign", "hdiutil", "/usr/bin/unzip", "xcrun"}
	} else {
		// Every normal release builds Linux in a container because the macOS host
		// has no GTK4 headers for CGO to link against.
		tools = []string{"/usr/libexec/PlistBuddy", "SetFile", "codesign", "ditto", "docker", "hdiutil", "mise", "openssl", "security", "/usr/bin/unzip"}
		if !p.options.skipNotarize {
			tools = append(tools, "xcrun")
		}
	}
	if !p.options.skipUpload {
		// gh records the release on GitHub after upload; check it here so a missing
		// or unauthenticated gh aborts before any build or the irreversible upload.
		tools = append(tools, "curl", "gh")
	}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("required tool %s: %w", tool, err)
		}
	}
	if !p.options.skipUpload {
		output, err := commandOutput(ctx, "curl", "--help", "all")
		if err != nil {
			return err
		}
		if !strings.Contains(output, "--aws-sigv4") {
			return errors.New("curl with --aws-sigv4 support is required (curl >= 7.86)")
		}
		if err := quietCommand(ctx, "gh", "auth", "status"); err != nil {
			return fmt.Errorf("gh must be authenticated to record the GitHub release: %w", err)
		}
	}
	return nil
}

func (p *publisher) cleanup() error {
	var cleanupErr error
	if p.keychainPath != "" {
		args := append([]string{"list-keychains", "-d", "user", "-s"}, p.originalKeychains...)
		if err := quietCommand(context.Background(), "security", args...); err != nil {
			cleanupErr = err
		}
		_ = quietCommand(context.Background(), "security", "delete-keychain", p.keychainPath)
	}
	if err := os.RemoveAll(p.workDir); cleanupErr == nil && err != nil {
		cleanupErr = err
	}
	return cleanupErr
}

func (p *publisher) build(ctx context.Context) error {
	fmt.Println("==> building universal .app")
	command := exec.CommandContext(ctx, "mise", "x", "--", "wails3", "task", "darwin:package:universal")
	command.Dir = desktopDir
	command.Env = append(os.Environ(),
		"HIVE_DESKTOP_VERSION="+p.options.version.String(),
		"HIVE_DESKTOP_COMMIT="+p.commit,
		"HIVE_DESKTOP_DATE="+p.buildDate,
	)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("build universal app: %w", err)
	}
	app := filepath.Join(desktopDir, "bin", "Hive.app")
	if info, err := os.Stat(app); err != nil || !info.IsDir() {
		return errors.New("build did not produce cmd/desktop/bin/Hive.app")
	}
	base := p.options.version.base
	baseText := fmt.Sprintf("%d.%d.%d", base.major, base.minor, base.patch)
	plistPath := filepath.Join(app, "Contents", "Info.plist")
	for _, key := range []string{"CFBundleShortVersionString", "CFBundleVersion"} {
		if err := runCommand(ctx, "/usr/libexec/PlistBuddy", "-c", "Set :"+key+" "+baseText, plistPath); err != nil {
			return err
		}
	}
	return nil
}

func gitHead(ctx context.Context) (string, error) {
	output, err := commandOutput(ctx, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve build commit: %w", err)
	}
	return strings.TrimSpace(output), nil
}

func (p *publisher) sign(ctx context.Context) error {
	fmt.Println("==> importing Developer ID certificate into an ephemeral keychain")
	p.keychainPath = filepath.Join(p.workDir, "signing.keychain-db")
	password, err := commandOutput(ctx, "openssl", "rand", "-base64", "24")
	if err != nil {
		return err
	}
	password = strings.TrimSpace(password)
	keychains, err := commandOutput(ctx, "security", "list-keychains", "-d", "user")
	if err != nil {
		return err
	}
	for line := range strings.Lines(keychains) {
		if path := strings.Trim(strings.TrimSpace(line), `"`); path != "" {
			p.originalKeychains = append(p.originalKeychains, path)
		}
	}
	certPath := filepath.Join(p.workDir, "cert.p12")
	if err := decodeSecret(p.options.signCertificate, certPath); err != nil {
		return fmt.Errorf("decode MACOS_CERTIFICATE: %w", err)
	}
	if err := runCommand(ctx, "security", "create-keychain", "-p", password, p.keychainPath); err != nil {
		return err
	}
	if err := runCommand(ctx, "security", "set-keychain-settings", "-lut", "3600", p.keychainPath); err != nil {
		return err
	}
	if err := runCommand(ctx, "security", "unlock-keychain", "-p", password, p.keychainPath); err != nil {
		return err
	}
	if err := runCommand(ctx, "security", "import", certPath, "-P", p.options.signPassword, "-k", p.keychainPath, "-T", "/usr/bin/codesign"); err != nil {
		return err
	}
	if err := quietCommand(ctx, "security", "set-key-partition-list", "-S", "apple-tool:,apple:,codesign:", "-s", "-k", password, p.keychainPath); err != nil {
		return err
	}
	args := append([]string{"list-keychains", "-d", "user", "-s", p.keychainPath}, p.originalKeychains...)
	if err := runCommand(ctx, "security", args...); err != nil {
		return err
	}
	if err := os.Remove(certPath); err != nil {
		return err
	}

	fmt.Println("==> codesigning (Developer ID, hardened runtime)")
	app := filepath.Join(desktopDir, "bin", "Hive.app")
	if err := runCommand(ctx, "codesign", "--force", "--deep", "--timestamp", "--options", "runtime", "--entitlements", filepath.Join(desktopDir, "build", "darwin", "entitlements.plist"), "--sign", p.options.signIdentity, app); err != nil {
		return err
	}
	return runCommand(ctx, "codesign", "--verify", "--strict", "--verbose=2", app)
}

func decodeSecret(value, path string) error {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	return os.WriteFile(path, decoded, 0o600)
}

type notaryResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (p *publisher) notarize(ctx context.Context) error {
	fmt.Println("==> notarizing Hive.app")
	app := filepath.Join(desktopDir, "bin", "Hive.app")
	archive := filepath.Join(p.workDir, "notarize.zip")
	if err := runCommand(ctx, "ditto", "-c", "-k", "--keepParent", app, archive); err != nil {
		return err
	}
	return p.notarizeAndStaple(ctx, archive, app)
}

// notaryKeyFile materialises the App Store Connect key once per release, so a
// second notarization round does not decode the secret again.
func (p *publisher) notaryKeyFile() (string, error) {
	if p.notaryKeyPath == "" {
		path := filepath.Join(p.workDir, "ac_api_key.p8")
		if err := decodeSecret(p.options.notaryKey, path); err != nil {
			return "", fmt.Errorf("decode AC_API_KEY: %w", err)
		}
		p.notaryKeyPath = path
	}
	return p.notaryKeyPath, nil
}

// notarizeAndStaple submits one file to Apple's notary service, waits for a
// verdict, and staples the resulting ticket to target. The two paths differ for
// the app, which has to travel inside a zip but carries the ticket itself.
func (p *publisher) notarizeAndStaple(ctx context.Context, submit, target string) error {
	keyPath, err := p.notaryKeyFile()
	if err != nil {
		return err
	}
	credentials := []string{"--key", keyPath, "--key-id", p.options.notaryKeyID, "--issuer", p.options.notaryIssuerID}
	output, err := commandOutput(ctx, "xcrun", append([]string{"notarytool", "submit", submit, "--output-format", "json"}, credentials...)...)
	if err != nil {
		return err
	}
	var submission notaryResponse
	if err := json.Unmarshal([]byte(output), &submission); err != nil {
		return fmt.Errorf("decode notary submission: %w", err)
	}
	if submission.ID == "" {
		return errors.New("decode notary submission: missing id")
	}
	fmt.Printf("    submission: %s\n", submission.ID)

	deadline := time.Now().Add(2 * time.Hour)
	consecutiveErrors := 0
	for time.Now().Before(deadline) {
		output, err := commandOutput(ctx, "xcrun", append([]string{"notarytool", "info", submission.ID, "--output-format", "json"}, credentials...)...)
		if err != nil {
			consecutiveErrors++
			fmt.Fprintf(os.Stderr, "    status check failed (%d/10)\n", consecutiveErrors)
			if consecutiveErrors >= 10 {
				return errors.New("too many notarytool errors")
			}
		} else {
			consecutiveErrors = 0
			var info notaryResponse
			if err := json.Unmarshal([]byte(output), &info); err != nil {
				return fmt.Errorf("decode notarization status: %w", err)
			}
			fmt.Printf("    status: %s\n", info.Status)
			switch info.Status {
			case "Accepted":
				if err := runCommand(ctx, "xcrun", "stapler", "staple", target); err != nil {
					return err
				}
				return runCommand(ctx, "xcrun", "stapler", "validate", target)
			case "In Progress":
			case "Invalid", "Rejected":
				_ = runCommand(ctx, "xcrun", append([]string{"notarytool", "log", submission.ID}, credentials...)...)
				return fmt.Errorf("notarization failed: %s", info.Status)
			default:
				return fmt.Errorf("unexpected notarization status: %s", info.Status)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(30 * time.Second):
		}
	}
	return errors.New("timed out waiting for notarization")
}

func (p *publisher) packageApp(ctx context.Context) (releaseArtifact, error) {
	zipName := fmt.Sprintf("Hive-%s-darwin-universal.zip", p.options.version)
	zipPath := filepath.Join(desktopDir, "bin", zipName)
	fmt.Printf("==> packaging %s\n", zipName)
	// ditto must run in cmd/desktop/bin so the archive has Hive.app at its root.
	command := exec.CommandContext(ctx, "ditto", "-c", "-k", "--norsrc", "--noextattr", "--noacl", "--keepParent", "Hive.app", zipName)
	command.Dir = filepath.Join(desktopDir, "bin")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return releaseArtifact{}, fmt.Errorf("package app: %w", err)
	}
	checksum, size, err := fileChecksum(zipPath)
	if err != nil {
		return releaseArtifact{}, err
	}
	fmt.Printf("%s  %s\n", checksum, zipName)

	if err := p.verifyPackagedApp(ctx, zipPath); err != nil {
		return releaseArtifact{}, err
	}
	return releaseArtifact{
		platformKey: "darwin-universal",
		role:        updateArtifact,
		name:        zipName,
		path:        zipPath,
		checksum:    checksum,
		size:        size,
	}, nil
}

func (p *publisher) verifyPackagedApp(ctx context.Context, zipPath string) error {
	fmt.Println("==> verifying packaged app after plain ZIP extraction")
	extracted := filepath.Join(p.workDir, "extracted")
	if err := os.RemoveAll(extracted); err != nil {
		return err
	}
	if err := os.MkdirAll(extracted, 0o755); err != nil {
		return err
	}
	if err := runCommand(ctx, "/usr/bin/unzip", "-q", zipPath, "-d", extracted); err != nil {
		return err
	}
	extractedApp := filepath.Join(extracted, "Hive.app")
	if err := filepath.WalkDir(extractedApp, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), "._") {
			return fmt.Errorf("packaged app contains signature-breaking AppleDouble file: %s", path)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := verifyCommitStamp(filepath.Join(extractedApp, "Contents", "MacOS", "hive-desktop"), p.commit, "packaged macOS app"); err != nil {
		return err
	}
	if err := runCommand(ctx, "codesign", "--verify", "--deep", "--strict", "--verbose=2", extractedApp); err != nil {
		return err
	}
	if p.options.skipNotarize {
		return nil
	}
	return runCommand(ctx, "xcrun", "stapler", "validate", extractedApp)
}

func verifyCommitStamp(path, commit, label string) error {
	stamped, err := fileContains(path, commit)
	if err != nil {
		return err
	}
	if !stamped {
		return fmt.Errorf("%s does not carry commit %s", label, commit)
	}
	return nil
}

func fileChecksum(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func (p *publisher) upload(ctx context.Context, artifacts []releaseArtifact, currentManifests map[string]channelManifest) error {
	releasePrefix := "desktop/releases/" + p.options.version.String()
	objects := make([]releaseObject, 0, len(artifacts)+1)
	for _, artifact := range artifacts {
		objects = append(objects, releaseObject{
			key:          releasePrefix + "/" + artifact.name,
			path:         artifact.path,
			contentType:  artifactContentType(artifact.name),
			cacheControl: "public, max-age=31536000, immutable",
		})
	}
	objects = append(objects, releaseObject{
		key:          releasePrefix + "/SHA256SUMS",
		path:         filepath.Join(desktopDir, "bin", "SHA256SUMS"),
		contentType:  "text/plain",
		cacheControl: "public, max-age=31536000, immutable",
	})

	pending := make([]releaseObject, 0, len(objects))
	for _, object := range objects {
		exists, err := p.r2Exists(ctx, object.key)
		if err != nil {
			return err
		}
		matches := false
		if exists && p.options.resume {
			matches, err = p.r2ObjectMatches(ctx, object.key, object.path)
			if err != nil {
				return err
			}
		}
		action, err := chooseR2UploadAction(exists, matches, p.options.resume, p.options.force)
		if err != nil {
			return fmt.Errorf("release %s already has %s in the bucket (immutable): %w", p.options.version, filepath.Base(object.key), err)
		}
		if action == r2ReuseObject {
			fmt.Printf("==> reusing byte-identical R2 object: %s\n", object.key)
			continue
		}
		pending = append(pending, object)
	}

	if len(pending) > 0 {
		fmt.Printf("==> uploading artifacts to r2://%s/%s/\n", p.options.r2Bucket, releasePrefix)
	}
	for _, object := range pending {
		if err := p.r2Put(ctx, object.key, object.path, object.contentType, object.cacheControl); err != nil {
			return err
		}
	}

	platforms, err := platformManifests(p.options.downloadBase, releasePrefix, artifacts)
	if err != nil {
		return err
	}

	entry, err := notesFor(p.options.version)
	if err != nil {
		return err
	}

	pubDate := time.Now().UTC().Format(time.RFC3339)
	for _, channel := range manifestChannels {
		if p.options.resume && manifestAlreadyPublished(p.options.version, channel, currentManifests) {
			fmt.Printf("==> channel manifest already published: %s\n", channel)
			continue
		}
		fmt.Printf("==> writing channel manifest: %s\n", channel)
		manifest := channelManifest{
			Channel:   channel,
			Version:   p.options.version.String(),
			PubDate:   pubDate,
			Summary:   entry.Summary,
			Notes:     entry.Body,
			Platforms: platforms,
		}
		contents, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return err
		}
		manifestPath := filepath.Join(p.workDir, "latest-"+channel+".json")
		if err := os.WriteFile(manifestPath, append(contents, '\n'), 0o644); err != nil {
			return err
		}
		if err := p.r2Put(ctx, "desktop/channels/"+channel+"/latest.json", manifestPath, "application/json", "no-cache"); err != nil {
			return err
		}
	}
	fmt.Printf("Release %s published.\n", p.options.version)
	for _, artifact := range artifacts {
		fmt.Printf("  %s (%s): %s/%s/%s\n", artifact.platformKey, artifact.role, p.options.downloadBase, releasePrefix, artifact.name)
	}
	fmt.Printf("  manifests updated: %s\n", strings.Join(manifestChannels, " "))
	return nil
}

// platformManifests folds a release's artifacts into one manifest entry per
// platform. A platform's update and installer artifacts occupy different fields
// of the same entry, so the two never overwrite each other.
func platformManifests(downloadBase, releasePrefix string, artifacts []releaseArtifact) (map[string]platformManifest, error) {
	platforms := make(map[string]platformManifest)
	for _, artifact := range artifacts {
		url := fmt.Sprintf("%s/%s/%s", downloadBase, releasePrefix, artifact.name)
		entry := platforms[artifact.platformKey]
		switch artifact.role {
		case updateArtifact:
			entry.URL, entry.SHA256, entry.Size = url, artifact.checksum, artifact.size
		case installerArtifact:
			entry.InstallerURL, entry.InstallerSHA256, entry.InstallerSize = url, artifact.checksum, artifact.size
		default:
			return nil, fmt.Errorf("artifact %s has no role", artifact.name)
		}
		platforms[artifact.platformKey] = entry
	}
	for key, entry := range platforms {
		if err := validatePlatformManifest(key, entry); err != nil {
			return nil, err
		}
	}
	return platforms, nil
}

func artifactContentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".zip"):
		return "application/zip"
	case strings.HasSuffix(name, ".dmg"):
		return "application/x-apple-diskimage"
	default:
		return "application/gzip"
	}
}

func chooseR2UploadAction(exists, matches, resume, force bool) (r2UploadAction, error) {
	if !exists || force {
		return r2UploadObject, nil
	}
	if resume && matches {
		return r2ReuseObject, nil
	}
	if resume {
		return r2UploadObject, errors.New("remote object differs from the local resume artifact")
	}
	return r2UploadObject, errors.New("use --force to overwrite")
}

func (p *publisher) r2Endpoint(key string) string {
	return fmt.Sprintf("https://%s.r2.cloudflarestorage.com/%s/%s", p.options.r2AccountID, p.options.r2Bucket, key)
}

func (p *publisher) r2AuthArgs() []string {
	return []string{"--aws-sigv4", "aws:amz:auto:s3", "--user", p.options.r2AccessKey + ":" + p.options.r2SecretKey}
}

func r2CurlRetryArgs() []string {
	// Every R2 call is idempotent: HEAD/GET have no side effects and PUT writes
	// the same bytes to the same key. That makes curl's broad transient retry
	// safe here, including the TLS read failure that motivated resume support.
	//
	// Deliberately no --retry-max-time. curl starts that timer before the first
	// attempt, so any cap short enough to bound a fast HEAD would strip retries
	// from the 30MB+ artifact PUTs that need them most -- a transfer dying past
	// the cap gets zero. The attempt count is the bound.
	return []string{"--retry", "5", "--retry-all-errors", "--retry-delay", "2", "--connect-timeout", "30"}
}

func (p *publisher) r2Exists(ctx context.Context, key string) (bool, error) {
	args := []string{"--silent", "--show-error", "--output", "/dev/null", "--write-out", "%{http_code}", "--head"}
	args = append(args, r2CurlRetryArgs()...)
	args = append(args, p.r2AuthArgs()...)
	args = append(args, p.r2Endpoint(key))
	output, err := commandOutput(ctx, "curl", args...)
	if err != nil {
		return false, err
	}
	switch strings.TrimSpace(output) {
	case strconv.Itoa(200):
		return true, nil
	case strconv.Itoa(404):
		return false, nil
	default:
		return false, fmt.Errorf("check R2 object: HTTP %s", strings.TrimSpace(output))
	}
}

func (p *publisher) r2Put(ctx context.Context, key, path, contentType, cacheControl string) error {
	args := []string{"--fail", "--silent", "--show-error", "--request", "PUT", "--upload-file", path, "--header", "Content-Type: " + contentType, "--header", "Cache-Control: " + cacheControl}
	args = append(args, r2CurlRetryArgs()...)
	args = append(args, p.r2AuthArgs()...)
	args = append(args, p.r2Endpoint(key))
	return runCommand(ctx, "curl", args...)
}

func (p *publisher) r2ObjectMatches(ctx context.Context, key, localPath string) (bool, error) {
	remotePath := filepath.Join(p.workDir, "remote-"+filepath.Base(key))
	if err := os.Remove(remotePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	args := []string{"--fail", "--silent", "--show-error", "--output", remotePath}
	args = append(args, r2CurlRetryArgs()...)
	args = append(args, p.r2AuthArgs()...)
	args = append(args, p.r2Endpoint(key))
	if err := runCommand(ctx, "curl", args...); err != nil {
		return false, fmt.Errorf("download existing R2 object %s: %w", key, err)
	}
	remoteChecksum, remoteSize, err := fileChecksum(remotePath)
	if err != nil {
		return false, err
	}
	localChecksum, localSize, err := fileChecksum(localPath)
	if err != nil {
		return false, err
	}
	return remoteChecksum == localChecksum && remoteSize == localSize, nil
}

func runCommand(ctx context.Context, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

func quietCommand(ctx context.Context, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("%s failed: %s", name, strings.TrimSpace(string(output)))
	}
	return nil
}
