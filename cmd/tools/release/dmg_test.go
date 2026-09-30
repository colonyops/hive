package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestWriteDiskImageProducesInstallerLayout builds a real disk image from a
// stub app and mounts it, because the drag-to-install layout is only observable
// on the mounted volume — nothing in the build commands fails if the symlink
// lands as a directory or the volume icon is dropped.
//
// Releases are cut from a Mac (decision 0028), so this covers the machine that
// actually builds the installer; the Linux CI runner skips it.
func TestWriteDiskImageProducesInstallerLayout(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("disk images are a macOS format")
	}
	for _, tool := range []string{"hdiutil", "SetFile", "ditto"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is unavailable: %v", tool, err)
		}
	}

	root := t.TempDir()
	t.Chdir(root)
	stubApp := filepath.Join(desktopDir, "bin", "Hive.app", "Contents", "MacOS")
	if err := os.MkdirAll(stubApp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stubApp, "Hive"), []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	icon := filepath.Join(desktopDir, "build", "darwin", "icons.icns")
	if err := os.MkdirAll(filepath.Dir(icon), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(icon, []byte("icns"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	p := &publisher{workDir: t.TempDir()}
	staging := filepath.Join(p.workDir, "dmg")
	if err := stageInstaller(ctx, staging); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(root, "installer.dmg")
	if err := p.writeDiskImage(ctx, staging, image); err != nil {
		t.Fatal(err)
	}

	mount := filepath.Join(p.workDir, "assert-mnt")
	if err := os.MkdirAll(mount, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runCommand(ctx, "hdiutil", "attach", image, "-mountpoint", mount, "-nobrowse", "-noautoopen", "-readonly", "-quiet"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := detachVolume(context.Background(), mount); err != nil {
			t.Error(err)
		}
	})
	if err := verifyInstallerLayout(mount); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyInstallerLayoutRejectsACopiedDropTarget(t *testing.T) {
	t.Parallel()

	mount := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mount, "Hive.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mount, ".VolumeIcon.icns"), []byte("icns"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(mount, "Applications"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := verifyInstallerLayout(mount); err == nil {
		t.Fatal("verifyInstallerLayout accepted a drop target that is a directory, not a symlink")
	}
}
