package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTarball builds a gzipped tar with the given entries, for the negative
// cases writeBinaryTarball cannot itself produce.
func writeTarball(t *testing.T, path string, headers []*tar.Header) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	zip := gzip.NewWriter(file)
	archive := tar.NewWriter(zip)
	for _, header := range headers {
		if err := archive.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := archive.Write(make([]byte, header.Size)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zip.Close(); err != nil {
		t.Fatal(err)
	}
}

// The updater renames the archive's single entry straight over the running
// binary, so the packaged shape is a correctness requirement, not a convention.
func TestWriteBinaryTarballProducesOneExecutableEntry(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	binary := filepath.Join(dir, "hive-desktop")
	if err := os.WriteFile(binary, []byte("ELF-ish payload"), 0o755); err != nil {
		t.Fatal(err)
	}
	tarball := filepath.Join(dir, "out.tar.gz")
	if err := writeBinaryTarball(tarball, binary, "hive-desktop", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := verifyBinaryTarball(tarball, "hive-desktop"); err != nil {
		t.Fatalf("freshly written tarball failed verification: %v", err)
	}

	file, err := os.Open(tarball)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	zip, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	archive := tar.NewReader(zip)
	header, err := archive.Next()
	if err != nil {
		t.Fatal(err)
	}
	if header.Name != "hive-desktop" {
		t.Fatalf("entry name = %q, want hive-desktop", header.Name)
	}
	if header.FileInfo().Mode().Perm()&0o111 == 0 {
		t.Fatalf("entry mode %v lost the executable bit", header.FileInfo().Mode())
	}
	if _, err := archive.Next(); err == nil {
		t.Fatal("archive has a second entry; the updater rejects multi-entry archives")
	}
}

func TestVerifyBinaryTarballRejectsBadShapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		headers []*tar.Header
		want    string
	}{
		{
			name: "two entries",
			headers: []*tar.Header{
				{Name: "hive-desktop", Mode: 0o755, Size: 1, Typeflag: tar.TypeReg},
				{Name: "README", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg},
			},
			want: "exactly one entry",
		},
		{
			// A wrapper directory would make the helper rename a directory over
			// the executable path.
			name:    "directory instead of a binary",
			headers: []*tar.Header{{Name: "hive-desktop", Mode: 0o755, Typeflag: tar.TypeDir}},
			want:    "not a regular file",
		},
		{
			name:    "wrong entry name",
			headers: []*tar.Header{{Name: "hive", Mode: 0o755, Size: 1, Typeflag: tar.TypeReg}},
			want:    "want \"hive-desktop\"",
		},
		{
			name:    "not executable",
			headers: []*tar.Header{{Name: "hive-desktop", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}},
			want:    "not executable",
		},
		{
			name:    "empty archive",
			headers: nil,
			want:    "is empty",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "out.tar.gz")
			writeTarball(t, path, testCase.headers)
			err := verifyBinaryTarball(path, "hive-desktop")
			if err == nil {
				t.Fatal("expected verification to fail")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error %q does not mention %q", err, testCase.want)
			}
		})
	}
}

func TestTarballContainsDetectsCommitStamp(t *testing.T) {
	t.Parallel()

	const commit = "3807c2bb3cad64e9230ec70a9e01aee64cc1593b"
	dir := t.TempDir()
	binary := filepath.Join(dir, "hive-desktop")
	if err := os.WriteFile(binary, []byte("binary "+commit), 0o755); err != nil {
		t.Fatal(err)
	}
	tarball := filepath.Join(dir, "hive.tar.gz")
	if err := writeBinaryTarball(tarball, binary, linuxBinaryName, 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := tarballContains(tarball, linuxBinaryName, commit)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("stamped commit not found in packaged binary")
	}
	found, err = tarballContains(tarball, linuxBinaryName, strings.Repeat("0", 40))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("unstamped commit reported as present")
	}
}

// A build that lost its ldflags reports "dev", which releaseChannel rejects, so
// the app would ship with its updater permanently disabled. buildLinux greps
// the binary for the release commit to catch that.
func TestFileContainsDetectsCommitStamp(t *testing.T) {
	t.Parallel()

	const commit = "3807c2bb3cad64e9230ec70a9e01aee64cc1593b"
	path := filepath.Join(t.TempDir(), "binary")
	if err := os.WriteFile(path, []byte("\x00\x01padding"+commit+"padding\x00"), 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := fileContains(path, commit)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("stamped commit not found in binary")
	}
	found, err = fileContains(path, "0000000000000000000000000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("unstamped commit reported as present")
	}
}
