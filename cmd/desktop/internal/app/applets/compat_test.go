package applets

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseReadsForward(t *testing.T) {
	t.Run("an unknown key warns at any depth and the rest loads", func(t *testing.T) {
		a, err := Parse("x", []byte("version: 1\ntitle: T\nsidebar: {entry: s.html}\ntitlebar: {side: left}\ncommands:\n  - {id: a, title: A, chord: x}\n  - {id: b, title: B, chord: y}\n"), "")
		require.NoError(t, err)
		assert.Equal(t, []string{
			`this version of Hive ignores "commands.chord"`,
			`this version of Hive ignores "sidebar"`,
			`this version of Hive ignores "titlebar.side"`,
		}, a.Warnings)
		assert.Equal(t, "index.html", a.Entry)
	})
	t.Run("an unknown icon falls back to the default", func(t *testing.T) {
		a, err := Parse("x", []byte("version: 1\ntitle: T\nicon: laser\ncommands:\n  - {id: a, title: A, icon: laser}\n"), "")
		require.NoError(t, err)
		assert.Empty(t, a.Icon)
		assert.Empty(t, a.Commands[0].Icon)
		assert.Len(t, a.Warnings, 1)
	})
	t.Run("a hostile name never reaches a warning", func(t *testing.T) {
		a, err := Parse("x", []byte("version: 1\ntitle: T\nicon: \"a\\u202Eb\"\n\"k\\e[31m\": 1\n"), "")
		require.NoError(t, err)
		for _, warning := range a.Warnings {
			assert.True(t, isPlainText(warning), "%q", warning)
			assert.NotContains(t, warning, "\u202e")
		}
		assert.Len(t, a.Warnings, 2)
	})
	t.Run("warnings are capped", func(t *testing.T) {
		var body strings.Builder
		body.WriteString("version: 1\ntitle: T\n")
		for i := range 500 {
			fmt.Fprintf(&body, "key%d: 1\n", i)
		}
		a, err := Parse("x", []byte(body.String()), "")
		require.NoError(t, err)
		assert.Len(t, a.Warnings, maxWarnings+1)
		assert.Equal(t, "and 480 more", a.Warnings[maxWarnings])
	})
	t.Run("the wrong shape under a known key stays an error", func(t *testing.T) {
		for _, body := range []string{
			"version: 1\ntitle: T\nwidth: wide\n",
			"version: 1\ntitle: T\ncommands: nope\n",
			"version: 1\ntitle: T\nentry: ../x.html\n",
			"version: 1\ntitle: T\ncommands:\n  - {id: a, title: A}\n  - {id: a, title: B}\n",
			"version: 1\ntitle: a\ntitle: b\n",
		} {
			_, err := Parse("x", []byte(body), "")
			assert.Error(t, err, body)
		}
	})
}

func TestParseSaysWhenHiveIsTooOld(t *testing.T) {
	tooOld := func(t *testing.T, body, host, want string) {
		t.Helper()
		_, err := Parse("x", []byte(body), host)
		var update *UpdateHiveError
		require.ErrorAs(t, err, &update)
		assert.Contains(t, err.Error(), want)
	}
	t.Run("a later format", func(t *testing.T) {
		tooOld(t, "version: 2\ntitle: T\n", "0.59.0", "Update Hive")
		tooOld(t, "version: 2\nwidth: [a, new, shape]\n", "", "file format 2")
	})
	t.Run("a later Hive", func(t *testing.T) {
		tooOld(t, "version: 1\nhive: \">=0.60\"\ntitle: T\n", "0.59.9", "needs Hive 0.60 or later, and this is Hive 0.59.9")
		tooOld(t, "version: 1\nhive: \">=0.60.1\"\ntitle: T\n", "v0.60.0", "needs Hive 0.60.1")
		tooOld(t, "version: 1\nhive: \">=1.0\"\nwidth: wide\n", "0.60.0", "needs Hive 1.0")
	})
	t.Run("this Hive or a later one runs it", func(t *testing.T) {
		for _, host := range []string{"0.60.0", "0.60.1", "1.0.0", "v0.61.0-rc1", "dev", ""} {
			_, err := Parse("x", []byte("version: 1\nhive: \">=0.60\"\ntitle: T\n"), host)
			assert.NoError(t, err, host)
		}
	})
	t.Run("an unreadable requirement refuses rather than runs", func(t *testing.T) {
		for _, value := range []string{"\"0.60\"", "\"^1\"", "\">=1\"", "[a]", "\">=99999999999999999999.0\""} {
			_, err := Parse("x", []byte("version: 1\ntitle: T\nhive: "+value+"\n"), "0.60.0")
			assert.ErrorContains(t, err, "hive must look like", value)
		}
	})
}

func TestUnpackGivesTheUpdateMessageAsIs(t *testing.T) {
	_, err := Unpack(buildZip(t, manifestEntry("radio"),
		zipEntry{name: "files/applet.yaml", body: "version: 2\ntitle: T\n"},
		zipEntry{name: "files/index.html", body: "x"}), "")
	require.ErrorContains(t, err, "Update Hive")
	assert.NotContains(t, err.Error(), "not valid")
}

func TestUnpackIgnoresUnknownManifestFields(t *testing.T) {
	archive, err := Unpack(buildZip(t,
		zipEntry{name: ArchiveManifestName, body: `{"format":1,"id":"radio","hive":"9.9.9","origin":"../x","nested":{"a":[1]}}`},
		zipEntry{name: "files/applet.yaml", body: archiveYAML},
		zipEntry{name: "files/index.html", body: "x"},
		zipEntry{name: "files/bar.html", body: "x"}), "")
	require.NoError(t, err)
	assert.Equal(t, "radio", archive.Applet.ID)
}

func TestUnpackTakesAFilesFolderEntry(t *testing.T) {
	_, err := Unpack(buildZip(t, manifestEntry("radio"),
		zipEntry{name: "files/"},
		zipEntry{name: "files/applet.yaml", body: "version: 1\ntitle: T\n"},
		zipEntry{name: "files/index.html", body: "x"}), "")
	assert.NoError(t, err)
}

func TestUnpackBoundsWhatTheDialogShows(t *testing.T) {
	var many strings.Builder
	many.WriteString("version: 1\ntitle: T\ncommands:\n")
	for i := range maxArchiveCommands + 1 {
		fmt.Fprintf(&many, "  - {id: c%d, title: Open Settings}\n", i)
	}
	for name, body := range map[string]string{
		"too many commands":  many.String(),
		"long command title": "version: 1\ntitle: T\ncommands:\n  - {id: a, title: " + strings.Repeat("a", maxArchiveTitleRunes+1) + "}\n",
		"long keyword":       "version: 1\ntitle: T\ncommands:\n  - {id: a, title: A, keywords: [" + strings.Repeat("a", maxArchiveTitleRunes+1) + "]}\n",
		"many keywords":      "version: 1\ntitle: T\ncommands:\n  - {id: a, title: A, keywords: [" + strings.Repeat("a,", maxArchiveKeywords) + "a]}\n",
		"long cwd":           "version: 1\ntitle: T\ncwd: " + strings.Repeat("a", maxArchiveCwdBytes+1) + "\n",
		"huge file":          "version: 1\ntitle: T\n#" + strings.Repeat("a", maxArchiveYAML) + "\n",
		"bidi title":         "version: 1\ntitle: \"Safe\\u202Egnp\"\n",
		"control title":      "version: 1\ntitle: \"a\\u0007b\"\n",
		"line separator":     "version: 1\ntitle: \"a\\u2028b\"\n",
		"bidi command title": "version: 1\ntitle: T\ncommands:\n  - {id: a, title: \"a\\u2066b\"}\n",
		"bidi keyword":       "version: 1\ntitle: T\ncommands:\n  - {id: a, title: A, keywords: [\"a\\u202Db\"]}\n",
		"bidi cwd":           "version: 1\ntitle: T\ncwd: \"a\\u202Eb\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Unpack(buildZip(t, manifestEntry("radio"),
				zipEntry{name: "files/applet.yaml", body: body},
				zipEntry{name: "files/index.html", body: "x"}), "")
			assert.Error(t, err)
		})
	}
	t.Run("a bidi path", func(t *testing.T) {
		_, err := Unpack(buildZip(t, manifestEntry("radio"),
			zipEntry{name: "files/applet.yaml", body: "version: 1\ntitle: T\n"},
			zipEntry{name: "files/index.html", body: "x"},
			zipEntry{name: "files/a\u202egnp.js", body: "x"}), "")
		assert.ErrorContains(t, err, "unsafe path")
	})
	t.Run("an emoji title with a joiner is fine", func(t *testing.T) {
		_, err := Unpack(buildZip(t, manifestEntry("radio"),
			zipEntry{name: "files/applet.yaml", body: "version: 1\ntitle: \"🧑‍🚀 Radio\"\n"},
			zipEntry{name: "files/index.html", body: "x"}), "")
		assert.NoError(t, err)
	})
}

func TestPackRefusesWhatImportWouldRefuseByContent(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"long title":   {"applet.yaml": "version: 1\ntitle: " + strings.Repeat("a", maxArchiveTitleRunes+1) + "\n", "index.html": "x"},
		"no page":      {"applet.yaml": "version: 1\ntitle: T\n"},
		"broken yaml":  {"applet.yaml": "version: [", "index.html": "x"},
		"hidden entry": {"applet.yaml": "version: 1\ntitle: T\nentry: .p.html\n", ".p.html": "x"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeTree(t, dir, files)
			_, err := Pack(Applet{ID: "radio", Dir: dir}, "")
			assert.Error(t, err)
		})
	}
}

func TestListPackLeavesOutALogInAnyCase(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"applet.yaml": "version: 1\ntitle: T\n", "index.html": "x", "debug.LOG": "secret"})
	list, err := ListPack(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"debug.LOG"}, list.LeftOut)
}

func TestReadPackFileHoldsAgainstAChangeAfterTheList(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer func() { _ = root.Close() }()

	t.Run("a link swapped in reaches nothing outside", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "id_ed25519")
		require.NoError(t, os.WriteFile(outside, []byte("private key"), 0o600))
		require.NoError(t, os.Symlink(outside, filepath.Join(dir, "notes.txt")))
		_, err := readPackFile(root, "notes.txt", MaxUnpackedBytes)
		assert.Error(t, err)
	})
	t.Run("a FIFO swapped in does not hang", func(t *testing.T) {
		require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o600))
		_, err := readPackFile(root, "pipe", MaxUnpackedBytes)
		assert.ErrorContains(t, err, "not a regular file")
	})
	t.Run("a file that grew past the limit", func(t *testing.T) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "big.js"), make([]byte, 11), 0o600))
		_, err := readPackFile(root, "big.js", 10)
		assert.ErrorContains(t, err, "larger than the limit")
	})
}

func TestSurfacesAreCheckedOnImport(t *testing.T) {
	_, err := Unpack(buildZip(t, manifestEntry("radio"),
		zipEntry{name: "files/applet.yaml", body: "version: 1\ntitle: T\ntitlebar: {entry: bar.html}\n"},
		zipEntry{name: "files/index.html", body: "x"}), "")
	assert.ErrorContains(t, err, "the title bar")
}

func TestPackRecordsTheExportingVersionAndNothingReadsIt(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"applet.yaml": "version: 1\ntitle: T\n", "index.html": "x"})
	packed, err := Pack(Applet{ID: "radio", Dir: dir}, "0.59.2")
	require.NoError(t, err)
	reader, err := zip.NewReader(bytes.NewReader(packed.Data), int64(len(packed.Data)))
	require.NoError(t, err)
	data, err := readZipEntry(reader.File[0], maxArchiveManifest)
	require.NoError(t, err)
	var manifest map[string]any
	require.NoError(t, json.Unmarshal(data, &manifest))
	assert.Equal(t, "0.59.2", manifest["hive"])
	_, err = Unpack(packed.Data, "0.1.0")
	assert.NoError(t, err, "the exporter's version does not limit who can import")
}

func TestHiveRequirementHoldsHoweverTheFileSpellsIt(t *testing.T) {
	for name, body := range map[string]string{
		"plain":       "hive: \">=999.0\"\n",
		"merge":       "<<: {hive: \">=999.0\"}\n",
		"merge alias": "x-a: &a {hive: \">=999.0\"}\n<<: *a\n",
		"merge list":  "x-a: &a {hive: \">=999.0\"}\n<<: [*a]\n",
		"alias key":   "x-k: &k hive\n*k : \">=999.0\"\n",
		"binary key":  "!!binary aGl2ZQ==: \">=999.0\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse("x", []byte("version: 1\ntitle: T\n"+body), "0.60.0")
			var update *UpdateHiveError
			require.ErrorAs(t, err, &update)
		})
	}
	t.Run("a merged value this Hive cannot read refuses", func(t *testing.T) {
		_, err := Parse("x", []byte("version: 1\ntitle: T\n<<: {hive: garbage}\n"), "0.60.0")
		assert.ErrorIs(t, err, errBadMinimumHive)
	})
	t.Run("a merged later format says to update", func(t *testing.T) {
		_, err := Parse("x", []byte("title: T\n<<: {version: 2}\n"), "0.60.0")
		var update *UpdateHiveError
		require.ErrorAs(t, err, &update)
	})
	t.Run("a null refuses", func(t *testing.T) {
		_, err := Parse("x", []byte("version: 1\ntitle: T\nhive: null\n"), "0.60.0")
		assert.ErrorIs(t, err, errBadMinimumHive)
	})
}

func TestIgnoredKeysAreSeenThroughAnySpelling(t *testing.T) {
	warned := func(t *testing.T, body string) []string {
		t.Helper()
		a, err := Parse("x", []byte("version: 1\ntitle: T\n"+body), "")
		require.NoError(t, err)
		return a.Warnings
	}
	ignores := func(key string) string { return fmt.Sprintf("this version of Hive ignores %q", key) }

	assert.Contains(t, warned(t, "<<: {limit-network: true}\n"), ignores("limit-network"))
	assert.Contains(t, warned(t, "titlebar: {<<: {sandbox: true}}\n"), ignores("titlebar.sandbox"))
	assert.Contains(t, warned(t, "x-a: &a {sandbox: true}\n<<: [*a]\n"), ignores("sandbox"))
	assert.Contains(t, warned(t, "x-k: &k real-name\n*k : 1\n"), ignores("real-name"))
	assert.Contains(t, warned(t, "\"<<\": {limit-network: true}\n"), ignores("a key with an unusual name"), "quoted, this is a key and not a merge")
	assert.Contains(t, warned(t, "---\nversion: 1\ntitle: second\n"), "this file has a second YAML document, and Hive reads only the first")
	assert.Empty(t, warned(t, "<<: {icon: play}\n"), "a merged key Hive knows is not a warning")
}

func TestPackPathsRefuseWhatACaseInsensitiveDiskWouldMerge(t *testing.T) {
	require.ErrorContains(t, checkPackPaths([]PackedFile{{Path: "README.md"}, {Path: "Readme.md"}}), "differ only by case")
	require.ErrorContains(t, checkPackPaths([]PackedFile{{Path: "a"}, {Path: "A/b"}}), "file and a folder")
	assert.NoError(t, checkPackPaths([]PackedFile{{Path: "a.js"}, {Path: "b/a.js"}}))
}
