package applets

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/colonyops/hive/cmd/desktop/internal/app/slug"
)

const (
	// ArchiveFormat is the version of the file Pack writes. Unpack refuses any other, so
	// a later format can be told apart rather than misread.
	ArchiveFormat = 1
	// ArchiveExtension names a shared applet: a zip holding ArchiveManifestName and the
	// applet's files under files/.
	ArchiveExtension = ".hiveapplet"
	// ArchiveManifestName is the one entry outside files/.
	ArchiveManifestName = "hive-applet.json"

	MaxArchiveBytes = 32 << 20
	MaxArchiveFiles = 500
	// MaxUnpackedBytes bounds what an archive may expand to, which its own headers
	// cannot be trusted to report.
	MaxUnpackedBytes = 64 << 20

	archiveFilesPrefix   = "files/"
	maxArchivePathBytes  = 255
	maxArchivePathDepth  = 16
	maxArchiveManifest   = 4 << 10
	maxArchiveYAML       = 256 << 10
	maxArchiveTitleRunes = 120
	maxArchiveCwdBytes   = 1 << 10
	maxArchiveCommands   = 200
	maxArchiveKeywords   = 32
)

// ErrAppletExists reports that an id is taken. Nothing replaces an applet on import.
var ErrAppletExists = errors.New("an applet with that id already exists")

// ArchiveFile is one file of an applet, by its slash-separated path in the directory.
type ArchiveFile struct {
	Path string
	Data []byte
}

// Archive is an applet read out of a shared file, checked and still only in memory.
type Archive struct {
	Applet Applet
	Files  []ArchiveFile
}

// PackedFile is one file an export includes.
type PackedFile struct {
	Path  string
	Bytes int64
}

// PackList is what an export of a directory contains and what it leaves out.
type PackList struct {
	Files   []PackedFile
	LeftOut []string
}

// PackedApplet is an export, whole in memory, with what it holds.
type PackedApplet struct {
	Data []byte
	List PackList
}

// archiveManifest ignores fields it does not know when read, so format 1 can grow. Format
// rises only for a layout an older Hive would misread. Hive is the exporting version, for
// a bug report: nothing reads it.
type archiveManifest struct {
	Format int    `json:"format"`
	ID     string `json:"id"`
	Hive   string `json:"hive,omitempty"`
}

// isLocalState matches what an applet's directory holds for this machine rather than as
// part of the applet: its saved storage, hidden files such as .env and .git, and logs.
func isLocalState(name string) bool {
	return strings.HasPrefix(name, ".") || strings.EqualFold(filepath.Ext(name), ".log")
}

// ListPack walks an applet's directory for what Pack would write. It follows no symlink,
// since one could carry a file from anywhere on the machine into the export.
func ListPack(dir string) (PackList, error) {
	var list PackList
	var total int64
	err := filepath.WalkDir(dir, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if file == dir {
			return nil
		}
		rel, err := filepath.Rel(dir, file)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case isLocalState(entry.Name()):
			list.LeftOut = append(list.LeftOut, rel)
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		case entry.IsDir():
			return nil
		case !entry.Type().IsRegular():
			list.LeftOut = append(list.LeftOut, rel)
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := checkArchivePath(rel); err != nil {
			return err
		}
		total += info.Size()
		list.Files = append(list.Files, PackedFile{Path: rel, Bytes: info.Size()})
		return nil
	})
	if err != nil {
		return PackList{}, fmt.Errorf("applets: reading %s: %w", dir, err)
	}
	// What cannot be imported should fail here, for the person who can fix it.
	if len(list.Files) > MaxArchiveFiles {
		return PackList{}, fmt.Errorf("this applet has %d files, and the limit is %d", len(list.Files), MaxArchiveFiles)
	}
	if total > MaxUnpackedBytes {
		return PackList{}, fmt.Errorf("this applet is %d MB, and the limit is %d MB", total>>20, MaxUnpackedBytes>>20)
	}
	return list, nil
}

// Pack reads one applet's directory into an archive. It records no timestamp, owner or
// path from this machine: the entries are the relative paths and their contents. It runs
// the checks an import would, so what cannot be imported fails here, for the person who
// can fix it. The result is held in memory, so what the user reviews is what gets written.
func Pack(applet Applet, hiveVersion string) (PackedApplet, error) {
	list, err := ListPack(applet.Dir)
	if err != nil {
		return PackedApplet{}, err
	}
	root, err := os.OpenRoot(applet.Dir)
	if err != nil {
		return PackedApplet{}, fmt.Errorf("applets: reading %s: %w", applet.Dir, err)
	}
	defer func() { _ = root.Close() }()

	files := make([]ArchiveFile, 0, len(list.Files))
	remaining := int64(MaxUnpackedBytes)
	for i, file := range list.Files {
		data, err := readPackFile(root, file.Path, remaining)
		if err != nil {
			return PackedApplet{}, err
		}
		remaining -= int64(len(data))
		list.Files[i].Bytes = int64(len(data))
		files = append(files, ArchiveFile{Path: file.Path, Data: data})
	}
	if _, err := parseArchiveApplet(applet.ID, files, hiveVersion); err != nil {
		return PackedApplet{}, err
	}

	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	manifest, err := json.Marshal(archiveManifest{Format: ArchiveFormat, ID: applet.ID, Hive: hiveVersion})
	if err != nil {
		return PackedApplet{}, err
	}
	if err := writeZipEntry(archive, ArchiveManifestName, manifest); err != nil {
		return PackedApplet{}, err
	}
	for _, file := range files {
		if err := writeZipEntry(archive, archiveFilesPrefix+file.Path, file.Data); err != nil {
			return PackedApplet{}, err
		}
	}
	if err := archive.Close(); err != nil {
		return PackedApplet{}, err
	}
	if buf.Len() > MaxArchiveBytes {
		return PackedApplet{}, fmt.Errorf("this applet packs to %d MB, and the limit is %d MB", buf.Len()>>20, MaxArchiveBytes>>20)
	}
	return PackedApplet{Data: buf.Bytes(), List: list}, nil
}

// readPackFile reopens what ListPack saw, which a running page can have changed since. The
// root keeps a link from reaching outside the applet, O_NONBLOCK keeps a FIFO from hanging
// the export, and the limit holds against a file that grew.
func readPackFile(root *os.Root, rel string, limit int64) ([]byte, error) {
	file, err := root.OpenFile(filepath.FromSlash(rel), os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("applets: reading %s: %w", rel, err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("applets: reading %s: %w", rel, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file. Try the export again", rel)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("applets: reading %s: %w", rel, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("this applet is larger than the limit of %d MB", MaxUnpackedBytes>>20)
	}
	return data, nil
}

func writeZipEntry(archive *zip.Writer, name string, data []byte) error {
	entry, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return err
	}
	_, err = entry.Write(data)
	return err
}

// Unpack reads a shared file, which it treats as hostile. It writes nothing, and every
// problem refuses the whole file. A zip has no hard links, so the entry type check covers
// symlinks and every other special file. A warning from Parse carries only names it has
// already reduced to plain text.
func Unpack(raw []byte, hiveVersion string) (Archive, error) {
	if len(raw) > MaxArchiveBytes {
		return Archive{}, fmt.Errorf("this file is larger than the %d MB limit", MaxArchiveBytes>>20)
	}
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return Archive{}, fmt.Errorf("this is not a Hive applet file: %w", err)
	}
	// One more than the files, for the manifest.
	if len(reader.File) > MaxArchiveFiles+1 {
		return Archive{}, fmt.Errorf("this file has more than the limit of %d files", MaxArchiveFiles)
	}

	var manifest []byte
	var files []ArchiveFile
	seen := map[string]bool{}
	remaining := int64(MaxUnpackedBytes)
	for _, entry := range reader.File {
		if entry.Name == ArchiveManifestName {
			if manifest != nil {
				return Archive{}, errors.New("this file has two manifests")
			}
			if manifest, err = readZipEntry(entry, maxArchiveManifest); err != nil {
				return Archive{}, err
			}
			continue
		}
		rel, ok := strings.CutPrefix(entry.Name, archiveFilesPrefix)
		if !ok {
			return Archive{}, fmt.Errorf("this file has an entry outside files/: %q", entry.Name)
		}
		// A zip made by hand with zip -r lists the folder itself.
		if rel == "" {
			continue
		}
		if err := checkArchivePath(strings.TrimSuffix(rel, "/")); err != nil {
			return Archive{}, err
		}
		if entry.Mode().IsDir() && strings.HasSuffix(rel, "/") {
			continue
		}
		if !entry.Mode().IsRegular() || strings.HasSuffix(rel, "/") {
			return Archive{}, fmt.Errorf("%q is not a regular file, and only regular files can be imported", rel)
		}
		// A case-insensitive disk would write these over one another.
		key := strings.ToLower(norm.NFC.String(rel))
		if seen[key] {
			return Archive{}, fmt.Errorf("this file has %q twice", rel)
		}
		seen[key] = true
		data, err := readZipEntry(entry, remaining)
		if err != nil {
			return Archive{}, err
		}
		remaining -= int64(len(data))
		files = append(files, ArchiveFile{Path: rel, Data: data})
	}
	if err := checkFilesAreNotDirectories(seen); err != nil {
		return Archive{}, err
	}
	if manifest == nil {
		return Archive{}, fmt.Errorf("this is not a Hive applet file: it has no %s", ArchiveManifestName)
	}
	id, err := parseArchiveManifest(manifest)
	if err != nil {
		return Archive{}, err
	}
	applet, err := parseArchiveApplet(id, files, hiveVersion)
	if err != nil {
		return Archive{}, err
	}
	slices.SortFunc(files, func(a, b ArchiveFile) int { return strings.Compare(a.Path, b.Path) })
	return Archive{Applet: applet, Files: files}, nil
}

func checkArchivePath(rel string) error {
	switch {
	case rel == "":
		return errors.New("this file has an entry with no name")
	case len(rel) > maxArchivePathBytes:
		return fmt.Errorf("this file has a path longer than %d bytes", maxArchivePathBytes)
	case strings.ContainsAny(rel, "\\\x00:") || !isPlainText(rel):
		return fmt.Errorf("this file has an unsafe path: %q", rel)
	case path.IsAbs(rel) || path.Clean(rel) != rel || !filepath.IsLocal(filepath.FromSlash(rel)):
		return fmt.Errorf("this file has a path outside the applet: %q", rel)
	}
	parts := strings.Split(rel, "/")
	if len(parts) > maxArchivePathDepth {
		return fmt.Errorf("this file has a path deeper than %d folders", maxArchivePathDepth)
	}
	if slices.ContainsFunc(parts, isLocalState) {
		return fmt.Errorf("this file has hidden or local data, which an applet file must not carry: %q", rel)
	}
	return nil
}

func checkFilesAreNotDirectories(files map[string]bool) error {
	for file := range files {
		for parent := path.Dir(file); parent != "."; parent = path.Dir(parent) {
			if files[parent] {
				return fmt.Errorf("this file uses %q as both a file and a folder", parent)
			}
		}
	}
	return nil
}

// readZipEntry stops at limit by what it read: the size in the entry's header is the
// sender's claim.
func readZipEntry(entry *zip.File, limit int64) ([]byte, error) {
	if entry.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("this file unpacks past the limit of %d MB", MaxUnpackedBytes>>20)
	}
	body, err := entry.Open()
	if err != nil {
		return nil, fmt.Errorf("this file cannot be read at %q: %w", entry.Name, err)
	}
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("this file cannot be read at %q: %w", entry.Name, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("this file unpacks past the limit of %d MB", MaxUnpackedBytes>>20)
	}
	return data, nil
}

func parseArchiveManifest(raw []byte) (string, error) {
	var manifest archiveManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", fmt.Errorf("this file's %s is not valid: %w", ArchiveManifestName, err)
	}
	switch {
	case manifest.Format > ArchiveFormat:
		return "", fmt.Errorf("this file is format %d, and this version of Hive reads format %d. Update Hive to import it", manifest.Format, ArchiveFormat)
	case manifest.Format != ArchiveFormat:
		return "", fmt.Errorf("this file's %s has no valid format", ArchiveManifestName)
	case !slug.Valid(manifest.ID):
		return "", fmt.Errorf("this file names an invalid applet id: %q", manifest.ID)
	}
	return manifest.ID, nil
}

func parseArchiveApplet(id string, files []ArchiveFile, hiveVersion string) (Applet, error) {
	has := func(name string) bool {
		return slices.ContainsFunc(files, func(file ArchiveFile) bool { return file.Path == name })
	}
	index := slices.IndexFunc(files, func(file ArchiveFile) bool { return file.Path == FileName })
	if index < 0 {
		return Applet{}, fmt.Errorf("this file has no %s", FileName)
	}
	if len(files[index].Data) > maxArchiveYAML {
		return Applet{}, fmt.Errorf("this file's %s is larger than %d KB", FileName, maxArchiveYAML>>10)
	}
	applet, err := Parse(id, files[index].Data, hiveVersion)
	if err != nil {
		if _, tooOld := errors.AsType[*UpdateHiveError](err); tooOld {
			return Applet{}, err
		}
		return Applet{}, fmt.Errorf("this file's %s is not valid: %w", FileName, err)
	}
	if err := checkArchiveText(applet); err != nil {
		return Applet{}, err
	}
	if !has(filepath.ToSlash(applet.Entry)) {
		return Applet{}, fmt.Errorf("this file has no page %q, which its %s opens", applet.Entry, FileName)
	}
	for _, surface := range Surfaces {
		if page, offered := surface.Page(applet); offered && !has(filepath.ToSlash(page)) {
			return Applet{}, fmt.Errorf("this file has no page %q, which its %s shows in %s", page, FileName, surface.Label)
		}
	}
	return applet, nil
}

// checkArchiveText bounds what the import dialog and the palette show. Those fields decide
// whether someone trusts the file, so nothing in them may stall the page, push the warning
// out of sight, or reorder what is on screen.
func checkArchiveText(applet Applet) error {
	if len(applet.Commands) > maxArchiveCommands {
		return fmt.Errorf("this file has more than %d commands", maxArchiveCommands)
	}
	if len(applet.Cwd) > maxArchiveCwdBytes || !isPlainText(applet.Cwd) {
		return errors.New("this file's cwd is too long or has characters that are not safe to show")
	}
	if err := checkArchiveTitle("title", applet.Title); err != nil {
		return err
	}
	if !isPlainText(applet.Entry) {
		return errors.New("this file's entry has characters that are not safe to show")
	}
	for _, command := range applet.Commands {
		if err := checkArchiveTitle("command title", command.Title); err != nil {
			return err
		}
		if len(command.Keywords) > maxArchiveKeywords {
			return fmt.Errorf("this file has a command with more than %d keywords", maxArchiveKeywords)
		}
		for _, keyword := range command.Keywords {
			if err := checkArchiveTitle("command keyword", keyword); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkArchiveTitle(what, text string) error {
	if len([]rune(text)) > maxArchiveTitleRunes {
		return fmt.Errorf("this file's %s is longer than %d characters", what, maxArchiveTitleRunes)
	}
	if !isPlainText(text) {
		return fmt.Errorf("this file's %s has characters that are not safe to show", what)
	}
	return nil
}

// isPlainText refuses control characters, line separators and the invisible format
// characters that reorder text. The zero width joiner stays, because emoji are built on it.
func isPlainText(text string) bool {
	if !utf8.ValidString(text) {
		return false
	}
	return !strings.ContainsFunc(text, func(r rune) bool {
		return r != '\u200d' && unicode.In(r, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp)
	})
}

// Install writes the archive as a new applet under root. The directory appears whole or
// not at all: it is written beside root, where the catalog does not look, and renamed in.
// Files get plain permissions, so the archive cannot mark anything executable.
func (a Archive) Install(root, id string) (err error) {
	if !slug.Valid(id) {
		return fmt.Errorf("%q is not a valid applet id", id)
	}
	if _, isBuiltin := loadBuiltins(root)[id]; isBuiltin {
		return ErrAppletExists
	}
	target := filepath.Join(root, id)
	if _, statErr := os.Lstat(target); !errors.Is(statErr, fs.ErrNotExist) {
		if statErr != nil {
			return statErr
		}
		return ErrAppletExists
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(root), ".applet-import-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(staging)
		}
	}()
	if err := os.Chmod(staging, 0o755); err != nil {
		return err
	}
	for _, file := range a.Files {
		dest := filepath.Join(staging, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := writeNewFile(dest, file.Data); err != nil {
			return err
		}
	}
	return os.Rename(staging, target)
}

func writeNewFile(dest string, data []byte) error {
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// FreeID returns id if nothing holds it, and otherwise the first free id-2, id-3 and so on.
func FreeID(root, id string) string {
	builtins := loadBuiltins(root)
	taken := func(candidate string) bool {
		if _, isBuiltin := builtins[candidate]; isBuiltin {
			return true
		}
		_, err := os.Lstat(filepath.Join(root, candidate))
		return !errors.Is(err, fs.ErrNotExist)
	}
	if !taken(id) {
		return id
	}
	for n := 2; ; n++ {
		suffix := fmt.Sprintf("-%d", n)
		base := id[:min(len(id), slug.MaxLen-len(suffix))]
		if candidate := strings.TrimRight(base, "-") + suffix; !taken(candidate) {
			return candidate
		}
	}
}
