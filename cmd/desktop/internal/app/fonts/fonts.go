// Package fonts enumerates the font families installed on this machine so the
// app's chrome and its terminals can be pointed at one of them.
//
// The webview cannot answer this itself: the Local Font Access API
// (queryLocalFonts) is Chromium-only, and macOS runs on WKWebView, so a picker
// built on it would be empty on the platform the app ships first. CSS can still
// *resolve* a local family by name — only enumeration is missing — so scanning
// here and handing the frontend a list of names is enough.
package fonts

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// The glyphs a family has to render at one width to count as monospace. Narrow,
// wide, and round shapes together separate a fixed-pitch face from a
// proportional one; a face missing any of them is not a terminal font.
var widthProbe = []rune{'i', 'l', 'M', 'W', '0', 'x', ' '}

// ppem is the size advances are measured at. Any value works — only whether the
// advances agree matters — but a large one keeps rounding from making a
// proportional face look uniform.
const ppem = fixed.Int26_6(2048 << 6)

var fontExtensions = map[string]bool{
	".ttf": true,
	".otf": true,
	".ttc": true,
	".otc": true,
}

// Families is one scan's result, split by what a picker can offer. Monospace is
// a subset of All: a fixed-pitch family is a legitimate choice for the UI face
// too, and someone running a terminal font everywhere is the reason this
// package exists at all.
type Families struct {
	All       []string
	Monospace []string
}

// Scanning parses every font file on the machine, so the result is cached for
// the process: a font installed while the app runs is picked up on the next
// launch, which is the same deal every terminal emulator offers.
type Lister struct {
	once     sync.Once
	families Families
	// Swapped by tests to count scans and to keep them off the host's fonts.
	scan func() Families
}

func NewLister() *Lister {
	return &Lister{scan: func() Families { return scan(searchPaths()) }}
}

// List returns the installed families, sorted and deduplicated.
//
// A machine with no readable font directory is not an error — it yields empty
// lists, and the caller falls back to the bundled faces.
func (l *Lister) List() Families {
	l.once.Do(func() { l.families = l.scan() })
	return l.families
}

func scan(roots []string) Families {
	paths := collect(roots)

	// One parse per file, and a file is 2-3MB for a patched Nerd Font, so the
	// walk is bounded by disk rather than CPU. A worker per core keeps a few
	// hundred fonts under a second without pinning the machine.
	workers := min(runtime.NumCPU(), len(paths))
	if workers == 0 {
		return Families{}
	}

	// Deduped as it is collected rather than through a result channel: a font
	// collection (.ttc) yields a family per face, so the number of results is
	// not bounded by the number of files and any buffer sized from one could
	// wedge a worker against a receiver that has not started draining.
	var mu sync.Mutex
	seen := make(map[string]bool, len(paths))

	queue := make(chan string)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for path := range queue {
				found := familiesIn(path)
				if len(found) == 0 {
					continue
				}
				mu.Lock()
				for _, f := range found {
					// A family whose weight files disagree counts as monospace
					// if any of them is: the italic of a fixed-pitch family
					// often is not, and dropping the family over that would
					// hide it from the terminal picker entirely.
					seen[f.name] = seen[f.name] || f.monospace
				}
				mu.Unlock()
			}
		})
	}
	for _, path := range paths {
		queue <- path
	}
	close(queue)
	wg.Wait()

	all := slices.Collect(maps.Keys(seen))
	slices.SortFunc(all, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	monospace := make([]string, 0, len(all))
	for _, name := range all {
		if seen[name] {
			monospace = append(monospace, name)
		}
	}
	return Families{All: all, Monospace: monospace}
}

func collect(roots []string) []string {
	var paths []string
	for _, root := range roots {
		// A missing directory is the norm — every platform list names paths
		// that only some installs have — so a walk error is skipped, not
		// reported.
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				if entry != nil && entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			if fontExtensions[strings.ToLower(filepath.Ext(path))] {
				paths = append(paths, path)
			}
			return nil
		})
	}
	return paths
}

type family struct {
	name      string
	monospace bool
}

// familiesIn returns the families a single font file declares. A collection
// (.ttc) carries several faces, and an unparsable file yields none.
func familiesIn(path string) []family {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	// Read-only, so a failed close has nothing to report and nothing to retry.
	defer func() { _ = file.Close() }()

	if collection, err := sfnt.ParseCollectionReaderAt(file); err == nil {
		var families []family
		for i := range collection.NumFonts() {
			face, err := collection.Font(i)
			if err != nil {
				continue
			}
			if found, ok := describe(face); ok {
				families = append(families, found)
			}
		}
		return families
	}

	face, err := sfnt.ParseReaderAt(file)
	if err != nil {
		return nil
	}
	if found, ok := describe(face); ok {
		return []family{found}
	}
	return nil
}

func describe(face *sfnt.Font) (family, bool) {
	name, ok := familyName(face)
	if !ok {
		return family{}, false
	}
	return family{name: name, monospace: isMonospace(face)}, true
}

func familyName(face *sfnt.Font) (string, bool) {
	var buf sfnt.Buffer
	// The typographic family is what groups the four weight/slant faces of one
	// family under a single name; NameIDFamily splits at four faces, so a
	// family shipping more than that would list "Foo" and "Foo Light"
	// separately. Not every font sets it, hence the fallback.
	for _, id := range []sfnt.NameID{sfnt.NameIDTypographicFamily, sfnt.NameIDFamily} {
		name, err := face.Name(&buf, id)
		if err != nil {
			continue
		}
		// A leading dot marks a macOS system-reserved face (".SF NS Mono").
		// The OS hides these from font pickers and CSS will not resolve one by
		// name, so offering it would be offering a dead option.
		if name = strings.TrimSpace(name); name != "" && !strings.HasPrefix(name, ".") {
			return name, true
		}
	}
	return "", false
}

// isMonospace trusts the post table's own claim first, then falls back to
// measuring.
//
// The measurement is not a belt-and-braces check, it is the load-bearing one:
// a Nerd Font patched face carries double-width icon glyphs and so reports
// isFixedPitch=0, and those are exactly the faces a terminal user installs.
// Filtering on the flag alone hides every one of them.
func isMonospace(face *sfnt.Font) bool {
	if post := face.PostTable(); post != nil && post.IsFixedPitch {
		return true
	}

	var buf sfnt.Buffer
	var width fixed.Int26_6
	for _, r := range widthProbe {
		index, err := face.GlyphIndex(&buf, r)
		// Glyph 0 is .notdef: the face has no glyph for this rune, so it cannot
		// be the terminal font either.
		if err != nil || index == 0 {
			return false
		}
		advance, err := face.GlyphAdvance(&buf, index, ppem, font.HintingNone)
		if err != nil || advance == 0 {
			return false
		}
		if width == 0 {
			width = advance
			continue
		}
		if advance != width {
			return false
		}
	}
	return width != 0
}

// searchPaths lists where the OS keeps fonts, most-specific last. Missing
// entries are skipped by the walk, so naming a path an install may not have
// costs nothing.
func searchPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	var paths []string
	switch runtime.GOOS {
	case "darwin":
		paths = []string{
			"/System/Library/Fonts",
			"/Library/Fonts",
		}
		if home != "" {
			paths = append(paths, filepath.Join(home, "Library", "Fonts"))
		}
	case "windows":
		if root := os.Getenv("SystemRoot"); root != "" {
			paths = append(paths, filepath.Join(root, "Fonts"))
		}
		// Per-user installs, which is where a font installed without admin
		// rights lands.
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			paths = append(paths, filepath.Join(local, "Microsoft", "Windows", "Fonts"))
		}
	default:
		paths = []string{
			"/usr/share/fonts",
			"/usr/local/share/fonts",
		}
		if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
			paths = append(paths, filepath.Join(dataHome, "fonts"))
		} else if home != "" {
			paths = append(paths, filepath.Join(home, ".local", "share", "fonts"))
		}
		if home != "" {
			paths = append(paths, filepath.Join(home, ".fonts"))
		}
	}
	return paths
}
