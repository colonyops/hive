package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// linuxArches are the Linux architectures every release publishes. A release
// covers all platforms at once, so the channel manifest it writes always names
// every key here plus darwin-universal.
var linuxArches = []string{"amd64", "arm64"}

// linuxBinaryName is both the built binary and the tarball's single entry.
const linuxBinaryName = "hive-desktop"

// buildLinux builds, packages, and verifies one Linux architecture, returning
// the artifact to upload and register.
//
// The build runs through cmd/desktop/scripts/build/build-linux-docker.sh, which compiles in
// a container mirroring the ubuntu-24.04 stack. That keeps one definition of how
// a Linux binary is produced — the container runs the same `wails3 task
// linux:build` a native Linux host would — and it is what lets a Mac publish
// Linux at all, since the build needs CGO against GTK4 headers macOS does not
// have.
func (p *publisher) buildLinux(ctx context.Context, arch string) (releaseArtifact, error) {
	fmt.Printf("==> building linux/%s binary\n", arch)
	command := exec.CommandContext(ctx, "./cmd/desktop/scripts/build/build-linux-docker.sh", "--arch="+arch)
	command.Env = append(os.Environ(),
		"HIVE_DESKTOP_VERSION="+p.options.version.String(),
		"HIVE_DESKTOP_COMMIT="+p.commit,
		"HIVE_DESKTOP_DATE="+p.buildDate,
	)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return releaseArtifact{}, fmt.Errorf("build linux/%s: %w", arch, err)
	}

	binary := filepath.Join(desktopDir, "bin", linuxBinaryName)
	info, err := os.Stat(binary)
	if err != nil {
		return releaseArtifact{}, fmt.Errorf("build linux/%s did not produce %s: %w", arch, binary, err)
	}

	// A build that lost its ldflags stamps reports version "dev", which
	// releaseChannel rejects — the app would ship with its updater permanently
	// disabled, and nobody would find out until an update failed to arrive
	// months later. The commit SHA is the needle rather than the version: with
	// -buildvcs=false it can only enter the binary through the same -X block,
	// while a bare stable version like "0.2.0" false-matches the dependency
	// versions Go embeds in build info.
	if err := verifyCommitStamp(binary, p.commit, "linux/"+arch+" binary"); err != nil {
		return releaseArtifact{}, fmt.Errorf("%w; the build's VERSION_LDFLAGS did not apply", err)
	}

	name := fmt.Sprintf("Hive-%s-linux-%s.tar.gz", p.options.version, arch)
	path := filepath.Join(desktopDir, "bin", name)
	fmt.Printf("==> packaging %s\n", name)
	if err := writeBinaryTarball(path, binary, linuxBinaryName, info.Mode()); err != nil {
		return releaseArtifact{}, err
	}
	if err := verifyBinaryTarball(path, linuxBinaryName); err != nil {
		return releaseArtifact{}, err
	}

	checksum, size, err := fileChecksum(path)
	if err != nil {
		return releaseArtifact{}, err
	}
	fmt.Printf("%s  %s\n", checksum, name)
	return releaseArtifact{
		platformKey: "linux-" + arch,
		role:        updateArtifact,
		name:        name,
		path:        path,
		checksum:    checksum,
		size:        size,
	}, nil
}

// writeBinaryTarball writes binary into dst as a gzipped tar holding exactly one
// entry, named entryName at the archive root.
//
// Built here rather than shelling out to tar because the shape is load-bearing:
// the updater rejects archives with more than one top-level entry, and its
// helper renames whatever it extracts straight over the running executable, so
// a wrapper directory would replace the binary with a directory. BSD tar also
// likes to add ._* AppleDouble entries for extended attributes, which would
// silently make the archive two-entry.
func writeBinaryTarball(dst, binary, entryName string, mode os.FileMode) error {
	contents, err := os.ReadFile(binary)
	if err != nil {
		return fmt.Errorf("read %s: %w", binary, err)
	}
	file, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer func() { _ = file.Close() }()

	zip := gzip.NewWriter(file)
	archive := tar.NewWriter(zip)
	header := &tar.Header{
		Name:     entryName,
		Mode:     int64(mode.Perm()),
		Size:     int64(len(contents)),
		Typeflag: tar.TypeReg,
		// Fixed so the same binary always produces byte-identical archives.
		ModTime: time.Unix(0, 0).UTC(),
	}
	if err := archive.WriteHeader(header); err != nil {
		return fmt.Errorf("write tar header: %w", err)
	}
	if _, err := archive.Write(contents); err != nil {
		return fmt.Errorf("write tar entry: %w", err)
	}
	if err := archive.Close(); err != nil {
		return fmt.Errorf("close tar: %w", err)
	}
	if err := zip.Close(); err != nil {
		return fmt.Errorf("close gzip: %w", err)
	}
	return file.Close()
}

// verifyBinaryTarball re-reads a packaged tarball and asserts the properties the
// updater depends on: exactly one entry, named entryName, a regular file, still
// executable. Checked against the bytes that will actually be uploaded rather
// than trusting the writer above.
func verifyBinaryTarball(path, entryName string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	zip, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	defer func() { _ = zip.Close() }()

	archive := tar.NewReader(zip)
	entries := 0
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		entries++
		if entries > 1 {
			return fmt.Errorf("%s must contain exactly one entry; the updater rejects multi-entry archives", path)
		}
		if header.Name != entryName {
			return fmt.Errorf("%s contains %q, want %q: the updater renames the entry over the running binary", path, header.Name, entryName)
		}
		if header.Typeflag != tar.TypeReg {
			return fmt.Errorf("%s entry %q is not a regular file", path, header.Name)
		}
		if header.FileInfo().Mode().Perm()&0o111 == 0 {
			return fmt.Errorf("%s entry %q is not executable", path, header.Name)
		}
	}
	if entries == 0 {
		return fmt.Errorf("%s is empty", path)
	}
	return nil
}

func tarballContains(path, entryName, needle string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()
	zip, err := gzip.NewReader(file)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	defer func() { _ = zip.Close() }()
	archive := tar.NewReader(zip)
	header, err := archive.Next()
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	if header.Name != entryName {
		return false, fmt.Errorf("%s contains %q, want %q", path, header.Name, entryName)
	}
	contents, err := io.ReadAll(archive)
	if err != nil {
		return false, fmt.Errorf("read %s entry %q: %w", path, entryName, err)
	}
	return bytes.Contains(contents, []byte(needle)), nil
}

// fileContains reports whether the file holds needle. Used to confirm a build
// stamp survived into the binary.
func fileContains(path, needle string) (bool, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	return bytes.Contains(contents, []byte(needle)), nil
}
