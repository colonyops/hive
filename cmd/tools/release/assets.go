package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	releaseRepo         = "colonyops/hive"
	releaseDownloadBase = "https://github.com/" + releaseRepo + "/releases/download"
	// GoReleaser attaches cliChecksumsAsset to every release it publishes. The
	// desktop creates the release first, so this asset, not the release, says
	// whether the publish workflow already ran.
	cliChecksumsAsset = "checksums.txt"
)

func releaseAssetBase(version releaseVersion) string {
	return releaseDownloadBase + "/" + version.tag()
}

// releaseFile is one local file the release attaches.
type releaseFile struct {
	name     string
	path     string
	checksum string
	size     int64
}

type releaseAsset struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

type gitHubRelease struct {
	Draft  bool           `json:"draft"`
	Assets []releaseAsset `json:"assets"`
}

// fetchGitHubRelease reads the release for tag through the REST API, which
// reports each asset's digest; `gh release view` does not.
func fetchGitHubRelease(ctx context.Context, tag string) (gitHubRelease, bool, error) {
	output, err := exec.CommandContext(ctx, "gh", "api", "repos/"+releaseRepo+"/releases/tags/"+tag).CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "HTTP 404") {
			return gitHubRelease{}, false, nil
		}
		return gitHubRelease{}, false, fmt.Errorf("read GitHub release %s: %s", tag, strings.TrimSpace(string(output)))
	}
	var release gitHubRelease
	if err := json.Unmarshal(output, &release); err != nil {
		return gitHubRelease{}, false, fmt.Errorf("decode GitHub release %s: %w", tag, err)
	}
	return release, true, nil
}

func (r gitHubRelease) asset(name string) (releaseAsset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return releaseAsset{}, false
}

func cliAssetsPublished(ctx context.Context, tag string) (bool, error) {
	release, found, err := fetchGitHubRelease(ctx, tag)
	if err != nil || !found {
		return false, err
	}
	_, ok := release.asset(cliChecksumsAsset)
	return ok, nil
}

type assetUploadAction int

const (
	assetUpload assetUploadAction = iota
	assetReplace
	assetReuse
)

// chooseAssetUpload keeps a published asset immutable: only resume may reuse a
// byte-identical one, and only --force may replace it. An asset GitHub never finished receiving is not a published
// object, so it is replaced without --force.
func chooseAssetUpload(existing releaseAsset, exists bool, checksum string, size int64, resume, force bool) (assetUploadAction, error) {
	if !exists {
		return assetUpload, nil
	}
	if existing.State != "uploaded" || force {
		return assetReplace, nil
	}
	matches := existing.Digest == "sha256:"+checksum && existing.Size == size
	switch {
	case resume && matches:
		return assetReuse, nil
	case resume:
		return assetUpload, errors.New("the published asset differs from the local resume artifact")
	default:
		return assetUpload, errors.New("use --force to overwrite")
	}
}

// ensureGitHubRelease creates the version's release from the tag, with every
// program's notes, if it does not exist yet. The publish workflow later adds
// the CLI's archives to this same release.
func (p *publisher) ensureGitHubRelease(ctx context.Context) (gitHubRelease, error) {
	tag := p.options.version.tag()
	release, found, err := fetchGitHubRelease(ctx, tag)
	if err != nil {
		return gitHubRelease{}, err
	}
	if found {
		// A draft's assets need authentication, so a manifest that named them
		// would break every update.
		if release.Draft {
			return gitHubRelease{}, fmt.Errorf("GitHub release %s is a draft; publish or delete it first", tag)
		}
		return release, nil
	}
	body, err := releaseNotesBody(p.options.version)
	if err != nil {
		return gitHubRelease{}, err
	}
	notesPath := filepath.Join(p.workDir, "release-notes.md")
	if err := os.WriteFile(notesPath, []byte(body), 0o644); err != nil {
		return gitHubRelease{}, err
	}
	fmt.Printf("==> creating GitHub release %s\n", tag)
	if err := runCommand(ctx, "gh", "release", "create", tag, "--repo", releaseRepo, "--verify-tag", "--title", tag, "--notes-file", notesPath); err != nil {
		return gitHubRelease{}, err
	}
	return gitHubRelease{}, nil
}

func (p *publisher) uploadAssets(ctx context.Context, release gitHubRelease, files []releaseFile) error {
	tag := p.options.version.tag()
	for _, file := range files {
		existing, exists := release.asset(file.name)
		action, err := chooseAssetUpload(existing, exists, file.checksum, file.size, p.options.resume, p.options.force)
		if err != nil {
			return fmt.Errorf("release %s already has %s on GitHub: %w", p.options.version, file.name, err)
		}
		args := []string{"release", "upload", tag, file.path, "--repo", releaseRepo}
		switch action {
		case assetReuse:
			fmt.Printf("==> reusing byte-identical GitHub asset: %s\n", file.name)
			continue
		case assetReplace:
			args = append(args, "--clobber")
		case assetUpload:
		}
		fmt.Printf("==> uploading %s to GitHub release %s\n", file.name, tag)
		if err := runCommand(ctx, "gh", args...); err != nil {
			return err
		}
	}
	return nil
}
