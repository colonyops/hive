package applets

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const archiveYAML = "version: 1\ntitle: Radio\n"

type zipEntry struct {
	name string
	body string
	mode fs.FileMode
}

func manifestEntry(id string) zipEntry {
	return zipEntry{name: ArchiveManifestName, body: fmt.Sprintf(`{"format":%d,"id":%q}`, ArchiveFormat, id)}
}

func buildZip(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		w, err := archive.CreateHeader(header)
		require.NoError(t, err)
		_, err = w.Write([]byte(entry.body))
		require.NoError(t, err)
	}
	require.NoError(t, archive.Close())
	return buf.Bytes()
}

func validEntries(extra ...zipEntry) []zipEntry {
	return append([]zipEntry{
		manifestEntry("radio"),
		{name: "files/applet.yaml", body: archiveYAML},
		{name: "files/index.html", body: "<!doctype html>"},
	}, extra...)
}

func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o600))
	}
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestRoundTripKeepsTheFilesAndLeavesLocalStateBehind(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "radio")
	writeTree(t, dir, map[string]string{
		"applet.yaml":        archiveYAML,
		"index.html":         "<!doctype html>",
		"lib/stations.js":    "export const s = 1",
		".localstorage.json": `{"secret":"1"}`,
		".env":               "TOKEN=1",
		".git/config":        "x",
		"matches.log":        "x",
	})
	outside := filepath.Join(t.TempDir(), "id_rsa")
	require.NoError(t, os.WriteFile(outside, []byte("private key"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "key")))

	applet, err := Parse("radio", []byte(archiveYAML), "")
	require.NoError(t, err)
	applet.Dir = dir

	list, err := ListPack(dir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{".localstorage.json", ".env", ".git", "matches.log", "key"}, list.LeftOut)

	packed, err := Pack(applet, "")
	require.NoError(t, err)
	for _, leaked := range []string{"private key", "TOKEN=1", "secret", dir} {
		assert.NotContains(t, unzipAll(t, packed.Data), leaked)
	}

	archive, err := Unpack(packed.Data, "")
	require.NoError(t, err)
	assert.Equal(t, "radio", archive.Applet.ID)

	root := t.TempDir()
	require.NoError(t, archive.Install(root, "radio"))
	body, err := os.ReadFile(filepath.Join(root, "radio", "lib", "stations.js"))
	require.NoError(t, err)
	assert.Equal(t, "export const s = 1", string(body))
	assert.NoFileExists(t, filepath.Join(root, "radio", ".localstorage.json"))
	info, err := os.Stat(filepath.Join(root, "radio", "index.html"))
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o644), info.Mode().Perm())

	store := NewStore(root, "")
	require.NoError(t, store.Reload())
	_, ok := store.Get("radio")
	assert.True(t, ok)
}

func unzipAll(t *testing.T, raw []byte) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	var all strings.Builder
	for _, entry := range reader.File {
		assert.Less(t, entry.Modified.Year(), 1981, "%s records when it was written", entry.Name)
		all.WriteString(entry.Name + "\n")
		data, err := readZipEntry(entry, MaxUnpackedBytes)
		require.NoError(t, err)
		all.Write(data)
	}
	return all.String()
}

func TestPackWritesTheFormatVersion(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"applet.yaml": archiveYAML, "index.html": "x"})
	packed, err := Pack(Applet{ID: "radio", Dir: dir}, "1.2.3")
	require.NoError(t, err)
	reader, err := zip.NewReader(bytes.NewReader(packed.Data), int64(len(packed.Data)))
	require.NoError(t, err)
	data, err := readZipEntry(reader.File[0], maxArchiveManifest)
	require.NoError(t, err)
	var manifest archiveManifest
	require.NoError(t, json.Unmarshal(data, &manifest))
	assert.Equal(t, archiveManifest{Format: ArchiveFormat, ID: "radio", Hive: "1.2.3"}, manifest)
}

func TestPackRefusesWhatImportWouldRefuse(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{"applet.yaml": archiveYAML}
	for i := range MaxArchiveFiles + 1 {
		files[fmt.Sprintf("f%d.js", i)] = "x"
	}
	writeTree(t, dir, files)
	_, err := ListPack(dir)
	require.ErrorContains(t, err, "limit")
}

func TestUnpackRefusesHostileFiles(t *testing.T) {
	for name, tc := range map[string]struct {
		entries []zipEntry
		want    string
	}{
		"parent traversal":       {validEntries(zipEntry{name: "files/../../evil.js"}), "outside the applet"},
		"traversal in a folder":  {validEntries(zipEntry{name: "files/a/../../evil.js"}), "outside the applet"},
		"unclean path":           {validEntries(zipEntry{name: "files/a//b.js"}), "outside the applet"},
		"absolute path":          {validEntries(zipEntry{name: "files//etc/passwd"}), "outside the applet"},
		"absolute outside files": {validEntries(zipEntry{name: "/etc/passwd"}), "outside files/"},
		"backslash traversal":    {validEntries(zipEntry{name: `files/..\..\evil.js`}), "unsafe path"},
		"windows drive":          {validEntries(zipEntry{name: "files/C:/evil.js"}), "unsafe path"},
		"nul in a name":          {validEntries(zipEntry{name: "files/a\x00.js"}), "unsafe path"},
		"symlink":                {validEntries(zipEntry{name: "files/link", body: "/etc/passwd", mode: fs.ModeSymlink | 0o777}), "not a regular file"},
		"named pipe":             {validEntries(zipEntry{name: "files/pipe", mode: fs.ModeNamedPipe | 0o644}), "not a regular file"},
		"device":                 {validEntries(zipEntry{name: "files/dev", mode: fs.ModeDevice | 0o644}), "not a regular file"},
		"seeded storage":         {validEntries(zipEntry{name: "files/.localstorage.json", body: "{}"}), "hidden or local data"},
		"hidden folder":          {validEntries(zipEntry{name: "files/.git/hooks/x", body: "x"}), "hidden or local data"},
		"entry outside files":    {validEntries(zipEntry{name: "evil.js"}), "outside files/"},
		"same path twice":        {validEntries(zipEntry{name: "files/index.html"}), "twice"},
		"same path by case":      {validEntries(zipEntry{name: "files/INDEX.html"}), "twice"},
		"file used as a folder":  {validEntries(zipEntry{name: "files/index.html/x.js"}), "both a file and a folder"},
		"two manifests":          {validEntries(manifestEntry("other")), "two manifests"},
		"too deep":               {validEntries(zipEntry{name: "files/" + strings.Repeat("a/", maxArchivePathDepth) + "x.js"}), "deeper"},
		"long path":              {validEntries(zipEntry{name: "files/" + strings.Repeat("a", maxArchivePathBytes+1)}), "longer"},
		"no manifest":            {[]zipEntry{{name: "files/applet.yaml", body: archiveYAML}}, "no hive-applet.json"},
		"manifest not json":      {[]zipEntry{{name: ArchiveManifestName, body: "{"}}, "not valid"},
		"newer format":           {[]zipEntry{{name: ArchiveManifestName, body: `{"format":99,"id":"radio"}`}}, "Update Hive"},
		"no format":              {[]zipEntry{{name: ArchiveManifestName, body: `{"id":"radio"}`}}, "no valid format"},
		"id with a path":         {[]zipEntry{{name: ArchiveManifestName, body: `{"format":1,"id":"../evil"}`}, {name: "files/applet.yaml", body: archiveYAML}}, "invalid applet id"},
		"no applet.yaml":         {[]zipEntry{manifestEntry("radio"), {name: "files/index.html", body: "x"}}, "no applet.yaml"},
		"yaml wrong shape":       {[]zipEntry{manifestEntry("radio"), {name: "files/applet.yaml", body: archiveYAML + "titlebar: nope\n"}}, "not valid"},
		"yaml not yaml":          {[]zipEntry{manifestEntry("radio"), {name: "files/applet.yaml", body: "{{{"}}, "not valid"},
		"missing entry page":     {[]zipEntry{manifestEntry("radio"), {name: "files/applet.yaml", body: archiveYAML}}, "no page"},
		"long title":             {[]zipEntry{manifestEntry("radio"), {name: "files/applet.yaml", body: "version: 1\ntitle: " + strings.Repeat("t", maxArchiveTitleRunes+1) + "\n"}, {name: "files/index.html"}}, "title is longer"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Unpack(buildZip(t, tc.entries...), "")
			require.ErrorContains(t, err, tc.want)
		})
	}
	t.Run("title bar page missing", func(t *testing.T) {
		_, err := Unpack(buildZip(t, manifestEntry("radio"),
			zipEntry{name: "files/applet.yaml", body: archiveYAML + "titlebar:\n  entry: bar.html\n"},
			zipEntry{name: "files/index.html"}), "")
		require.ErrorContains(t, err, "no page")
	})
	t.Run("not a zip", func(t *testing.T) {
		_, err := Unpack([]byte("just some text"), "")
		require.ErrorContains(t, err, "not a Hive applet file")
	})
}

func TestUnpackEnforcesItsLimits(t *testing.T) {
	t.Run("archive size", func(t *testing.T) {
		_, err := Unpack(make([]byte, MaxArchiveBytes+1), "")
		require.ErrorContains(t, err, "larger than")
	})
	t.Run("file count", func(t *testing.T) {
		entries := validEntries()
		for i := range MaxArchiveFiles {
			entries = append(entries, zipEntry{name: fmt.Sprintf("files/f%d.js", i)})
		}
		_, err := Unpack(buildZip(t, entries...), "")
		require.ErrorContains(t, err, "limit")
	})
	t.Run("a zip bomb past the total", func(t *testing.T) {
		body := strings.Repeat("0", MaxUnpackedBytes/2+1)
		_, err := Unpack(buildZip(t, validEntries(
			zipEntry{name: "files/a.bin", body: body},
			zipEntry{name: "files/b.bin", body: body},
		)...), "")
		require.ErrorContains(t, err, "unpacks past")
	})
	t.Run("a header that lies about the size", func(t *testing.T) {
		raw := buildZip(t, validEntries(zipEntry{name: "files/a.bin", body: strings.Repeat("0", 2048)})...)
		reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		require.NoError(t, err)
		var entry *zip.File
		for _, file := range reader.File {
			if file.Name == "files/a.bin" {
				entry = file
			}
		}
		require.NotNil(t, entry)
		entry.UncompressedSize64 = 1
		_, err = readZipEntry(entry, 1024)
		require.Error(t, err)
	})
	t.Run("a corrupt entry", func(t *testing.T) {
		raw := buildZip(t, validEntries(zipEntry{name: "files/a.bin", body: strings.Repeat("x", 4096)})...)
		reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		require.NoError(t, err)
		for _, file := range reader.File {
			if file.Name == "files/a.bin" {
				offset, err := file.DataOffset()
				require.NoError(t, err)
				raw[offset+4] ^= 0xff
			}
		}
		_, err = Unpack(raw, "")
		require.Error(t, err)
	})
}

func TestUnpackNeedsATitlebarOnlyToOffer(t *testing.T) {
	archive, err := Unpack(buildZip(t, manifestEntry("radio"),
		zipEntry{name: "files/applet.yaml", body: archiveYAML + "titlebar:\n  entry: bar.html\n"},
		zipEntry{name: "files/index.html"},
		zipEntry{name: "files/bar.html"}), "")
	require.NoError(t, err)
	assert.Equal(t, "bar.html", archive.Applet.Titlebar.Entry)
}

func TestInstallNeverReplacesAndLeavesNothingBehind(t *testing.T) {
	archive, err := Unpack(buildZip(t, validEntries()...), "")
	require.NoError(t, err)

	parent := t.TempDir()
	root := filepath.Join(parent, "applets")
	writeTree(t, filepath.Join(root, "radio"), map[string]string{"mine.txt": "keep me"})

	require.ErrorIs(t, archive.Install(root, "radio"), ErrAppletExists)
	require.ErrorIs(t, archive.Install(root, BuilderID), ErrAppletExists)
	require.Error(t, archive.Install(root, "../evil"))
	require.Error(t, archive.Install(root, "Bad Name"))
	body, err := os.ReadFile(filepath.Join(root, "radio", "mine.txt"))
	require.NoError(t, err)
	assert.Equal(t, "keep me", string(body))
	assert.ElementsMatch(t, []string{"applets"}, dirNames(t, parent), "no staging directory stays")

	require.NoError(t, archive.Install(root, "radio-2"))
	assert.FileExists(t, filepath.Join(root, "radio-2", "index.html"))
	assert.ElementsMatch(t, []string{"applets"}, dirNames(t, parent))
}

func TestInstallLeavesNothingWhenAWriteFails(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "applets")
	// Two files that collide on disk, which only a hand-built Archive can reach.
	archive := Archive{Files: []ArchiveFile{{Path: "a.js"}, {Path: "a.js"}}}
	require.Error(t, archive.Install(root, "radio"))
	assert.ElementsMatch(t, []string{"applets"}, dirNames(t, parent))
	assert.Empty(t, dirNames(t, root))
}

func TestInstallDoesNotFollowASymlinkAtTheTarget(t *testing.T) {
	archive, err := Unpack(buildZip(t, validEntries()...), "")
	require.NoError(t, err)
	root := t.TempDir()
	elsewhere := t.TempDir()
	require.NoError(t, os.Symlink(elsewhere, filepath.Join(root, "radio")))
	require.ErrorIs(t, archive.Install(root, "radio"), ErrAppletExists)
	assert.Empty(t, dirNames(t, elsewhere))
}

func TestFreeIDStepsPastWhatIsTaken(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"radio/applet.yaml": "x", "radio-2/applet.yaml": "x"})
	assert.Equal(t, "new", FreeID(root, "new"))
	assert.Equal(t, "radio-3", FreeID(root, "radio"))
	assert.Equal(t, BuilderID+"-2", FreeID(root, BuilderID))
}
