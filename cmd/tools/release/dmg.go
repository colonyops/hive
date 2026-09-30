package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// dmgVolumeName names the mounted volume: the Finder sidebar entry and the
// title of the window that opens when the image is double-clicked.
const dmgVolumeName = "Hive"

// packageInstaller builds the macOS installer disk image — Hive.app beside an
// /Applications symlink, so opening the download gives the drag-to-install
// window a zip of the bare app cannot.
//
// It runs after the app is notarized and stapled, and notarizes the image
// itself. Gatekeeper evaluates the container the user opens, so a stapled app
// inside an unnotarized image still warns; both tickets are needed, the image's
// for the download and the app's so the copy in /Applications validates without
// a network round trip.
//
// The checksum is taken last: stapling rewrites the image.
func (p *publisher) packageInstaller(ctx context.Context) (releaseArtifact, error) {
	name := fmt.Sprintf("Hive-%s-darwin-universal.dmg", p.options.version)
	path := filepath.Join(desktopDir, "bin", name)
	fmt.Printf("==> packaging %s\n", name)

	staging := filepath.Join(p.workDir, "dmg")
	if err := stageInstaller(ctx, staging); err != nil {
		return releaseArtifact{}, err
	}
	if err := p.writeDiskImage(ctx, staging, path); err != nil {
		return releaseArtifact{}, err
	}
	if err := runCommand(ctx, "codesign", "--force", "--timestamp", "--sign", p.options.signIdentity, path); err != nil {
		return releaseArtifact{}, err
	}
	if !p.options.skipNotarize {
		fmt.Println("==> notarizing the installer disk image")
		if err := p.notarizeAndStaple(ctx, path, path); err != nil {
			return releaseArtifact{}, err
		}
	}
	if err := p.verifyDiskImage(ctx, path); err != nil {
		return releaseArtifact{}, err
	}
	checksum, size, err := fileChecksum(path)
	if err != nil {
		return releaseArtifact{}, err
	}
	fmt.Printf("%s  %s\n", checksum, name)
	return releaseArtifact{
		platformKey: "darwin-universal",
		role:        installerArtifact,
		name:        name,
		path:        path,
		checksum:    checksum,
		size:        size,
	}, nil
}

// stageInstaller lays out what the volume will contain.
func stageInstaller(ctx context.Context, staging string) error {
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return fmt.Errorf("create installer staging directory: %w", err)
	}
	// ditto rather than a Go copy: it carries the extended attributes and
	// resource forks the code signature covers.
	if err := runCommand(ctx, "ditto", filepath.Join(desktopDir, "bin", "Hive.app"), filepath.Join(staging, "Hive.app")); err != nil {
		return err
	}
	if err := os.Symlink("/Applications", filepath.Join(staging, "Applications")); err != nil {
		return fmt.Errorf("stage the /Applications drop target: %w", err)
	}
	return runCommand(ctx, "ditto", filepath.Join(desktopDir, "build", "darwin", "icons.icns"), filepath.Join(staging, ".VolumeIcon.icns"))
}

// writeDiskImage builds the compressed image in two passes: a writable image
// first, so the volume's custom-icon flag can be set while it is mounted, then
// a compressed read-only copy. The flag lives in the volume's Finder info, not
// in anything hdiutil can set at create time, and without it .VolumeIcon.icns
// is an inert file and the volume mounts with the generic disk icon.
func (p *publisher) writeDiskImage(ctx context.Context, staging, dst string) error {
	writable := filepath.Join(p.workDir, "installer-rw.dmg")
	if err := runCommand(ctx, "hdiutil", "create", "-srcfolder", staging, "-volname", dmgVolumeName, "-fs", "HFS+", "-format", "UDRW", "-ov", "-quiet", writable); err != nil {
		return err
	}

	mount := filepath.Join(p.workDir, "installer-mnt")
	if err := os.MkdirAll(mount, 0o755); err != nil {
		return fmt.Errorf("create installer mount point: %w", err)
	}
	if err := runCommand(ctx, "hdiutil", "attach", writable, "-mountpoint", mount, "-nobrowse", "-noautoopen", "-quiet"); err != nil {
		return err
	}
	err := runCommand(ctx, "SetFile", "-a", "C", mount)
	if detachErr := detachVolume(ctx, mount); err == nil {
		err = detachErr
	}
	if err != nil {
		return err
	}

	return runCommand(ctx, "hdiutil", "convert", writable, "-format", "UDZO", "-imagekey", "zlib-level=9", "-ov", "-quiet", "-o", dst)
}

// verifyDiskImage mounts the finished image and checks what a user actually
// receives: the app is at the volume root, the drop target really points at
// /Applications, and the signature and ticket survived the round trip through
// HFS+ and compression.
func (p *publisher) verifyDiskImage(ctx context.Context, path string) error {
	fmt.Println("==> verifying the installer disk image")
	mount := filepath.Join(p.workDir, "verify-mnt")
	if err := os.MkdirAll(mount, 0o755); err != nil {
		return fmt.Errorf("create verification mount point: %w", err)
	}
	if err := runCommand(ctx, "hdiutil", "attach", path, "-mountpoint", mount, "-nobrowse", "-noautoopen", "-readonly", "-quiet"); err != nil {
		return err
	}
	err := inspectMountedInstaller(ctx, mount, !p.options.skipNotarize)
	if err == nil {
		err = verifyCommitStamp(filepath.Join(mount, "Hive.app", "Contents", "MacOS", "hive-desktop"), p.commit, "installer macOS app")
	}
	if detachErr := detachVolume(ctx, mount); err == nil {
		err = detachErr
	}
	if err != nil {
		return err
	}
	if p.options.skipNotarize {
		return nil
	}
	return runCommand(ctx, "xcrun", "stapler", "validate", path)
}

func inspectMountedInstaller(ctx context.Context, mount string, notarized bool) error {
	if err := verifyInstallerLayout(mount); err != nil {
		return err
	}
	app := filepath.Join(mount, "Hive.app")
	if err := runCommand(ctx, "codesign", "--verify", "--deep", "--strict", "--verbose=2", app); err != nil {
		return err
	}
	if !notarized {
		return nil
	}
	return runCommand(ctx, "xcrun", "stapler", "validate", app)
}

// verifyInstallerLayout asserts the shape a user sees on the mounted volume:
// the app at the root, and a drop target that is a symlink to /Applications
// rather than a copied directory — dragging into a copy would install nothing.
func verifyInstallerLayout(mount string) error {
	if info, err := os.Stat(filepath.Join(mount, "Hive.app")); err != nil || !info.IsDir() {
		return fmt.Errorf("installer image has no Hive.app at the volume root")
	}
	target, err := os.Readlink(filepath.Join(mount, "Applications"))
	if err != nil {
		return fmt.Errorf("installer image has no /Applications drop target: %w", err)
	}
	if target != "/Applications" {
		return fmt.Errorf("installer image drop target points at %q, want /Applications", target)
	}
	if _, err := os.Stat(filepath.Join(mount, ".VolumeIcon.icns")); err != nil {
		return fmt.Errorf("installer image has no volume icon: %w", err)
	}
	return nil
}

// detachVolume unmounts a volume, retrying because Spotlight indexing a
// freshly written image holds it busy for a few seconds; the last attempt
// forces it rather than failing a release over a transient claim.
func detachVolume(ctx context.Context, mount string) error {
	const attempts = 5
	var err error
	for attempt := range attempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
		args := []string{"detach", mount, "-quiet"}
		if attempt == attempts-1 {
			args = append(args, "-force")
		}
		if err = quietCommand(ctx, "hdiutil", args...); err == nil {
			return nil
		}
	}
	return fmt.Errorf("detach %s: %w", mount, err)
}
