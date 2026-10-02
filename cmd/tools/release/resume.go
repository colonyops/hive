package main

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
)

func (p *publisher) resumePublish(ctx context.Context) error {
	fmt.Printf("==> resuming %s from verified artifacts in cmd/desktop/bin\n", p.options.version)
	artifacts, err := p.loadReleaseArtifacts(ctx)
	if err != nil {
		return err
	}
	if err := p.writeChecksums(artifacts); err != nil {
		return err
	}
	fmt.Println("==> reconstructed SHA256SUMS from verified local artifacts")

	if err := validatePublishSource(ctx, p.options.version); err != nil {
		return err
	}
	manifests, err := readManifests(ctx)
	if err != nil {
		return err
	}
	platforms, err := platformManifests(p.options.downloadBase, "desktop/releases/"+p.options.version.String(), artifacts)
	if err != nil {
		return err
	}
	entry, err := notesFor(p.options.version)
	if err != nil {
		return err
	}
	if err := validateResumeManifests(p.options.version, manifests, entry.Summary, entry.Body, platforms); err != nil {
		return err
	}
	return p.upload(ctx, artifacts, manifests)
}

func (p *publisher) loadReleaseArtifacts(ctx context.Context) ([]releaseArtifact, error) {
	version := p.options.version.String()
	load := func(platformKey string, role artifactRole, name string) (releaseArtifact, error) {
		path := filepath.Join(desktopDir, "bin", name)
		checksum, size, err := fileChecksum(path)
		if err != nil {
			return releaseArtifact{}, fmt.Errorf("load resume artifact %s: %w", name, err)
		}
		return releaseArtifact{
			platformKey: platformKey,
			role:        role,
			name:        name,
			path:        path,
			checksum:    checksum,
			size:        size,
		}, nil
	}

	zipName := fmt.Sprintf("Hive-%s-darwin-universal.zip", version)
	macArtifact, err := load("darwin-universal", updateArtifact, zipName)
	if err != nil {
		return nil, err
	}
	if err := p.verifyPackagedApp(ctx, macArtifact.path); err != nil {
		return nil, fmt.Errorf("verify resume artifact %s: %w", zipName, err)
	}

	dmgName := fmt.Sprintf("Hive-%s-darwin-universal.dmg", version)
	installer, err := load("darwin-universal", installerArtifact, dmgName)
	if err != nil {
		return nil, err
	}
	if err := p.verifyDiskImage(ctx, installer.path); err != nil {
		return nil, fmt.Errorf("verify resume artifact %s: %w", dmgName, err)
	}

	artifacts := []releaseArtifact{macArtifact, installer}
	for _, arch := range linuxArches {
		name := fmt.Sprintf("Hive-%s-linux-%s.tar.gz", version, arch)
		artifact, err := load("linux-"+arch, updateArtifact, name)
		if err != nil {
			return nil, err
		}
		if err := verifyBinaryTarball(artifact.path, linuxBinaryName); err != nil {
			return nil, fmt.Errorf("verify resume artifact %s: %w", name, err)
		}
		stamped, err := tarballContains(artifact.path, linuxBinaryName, p.commit)
		if err != nil {
			return nil, fmt.Errorf("verify resume artifact %s: %w", name, err)
		}
		if !stamped {
			return nil, fmt.Errorf("verify resume artifact %s: linux/%s binary does not carry commit %s", name, arch, p.commit)
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func validateResumeManifests(version releaseVersion, manifests map[string]channelManifest, summary, notes string, platforms map[string]platformManifest) error {
	for _, channel := range manifestChannels {
		manifest, ok := manifests[channel]
		if !ok {
			continue
		}
		current, err := parseVersion(manifest.Version)
		if err != nil {
			return fmt.Errorf("%s manifest version %q: %w", channel, manifest.Version, err)
		}
		switch compareVersions(version, current) {
		case 1:
			continue
		case -1:
			return fmt.Errorf("resume candidate %s is older than the affected %s manifest at %s", version, channel, manifest.Version)
		}
		if manifest.Channel != channel || manifest.Summary != summary || manifest.Notes != notes || !maps.Equal(manifest.Platforms, platforms) {
			return fmt.Errorf("affected %s manifest already names %s with conflicting release metadata", channel, version)
		}
	}
	return nil
}

func manifestAlreadyPublished(version releaseVersion, channel string, manifests map[string]channelManifest) bool {
	manifest, ok := manifests[channel]
	return ok && strings.TrimSpace(manifest.Version) == version.String()
}
