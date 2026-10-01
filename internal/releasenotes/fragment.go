package releasenotes

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// UnreleasedDir holds the draft, one file per change, under the changelog
// directory. A pull request lands its note by adding a file no other branch is
// editing, which is what keeps concurrent work from conflicting over the
// changelog (ADR release-notes-accumulate-as-fragments).
const UnreleasedDir = "unreleased"

// Kind is the section a fragment renders under.
type Kind string

const (
	KindAdded   Kind = "added"
	KindChanged Kind = "changed"
	KindFixed   Kind = "fixed"
)

// Kinds lists every kind in the order the draft renders them.
var Kinds = []Kind{KindAdded, KindChanged, KindFixed}

// ParseKind accepts a kind as it is written in a fragment header.
func ParseKind(s string) (Kind, bool) {
	kind := Kind(strings.ToLower(strings.TrimSpace(s)))
	return kind, slices.Contains(Kinds, kind)
}

func (k Kind) heading() string {
	return strings.ToUpper(string(k)[:1]) + string(k)[1:]
}

// Fragment is one unreleased change.
type Fragment struct {
	// Name is the filename, which is what orders fragments within a section.
	Name string
	Kind Kind
	Body string
}

var fragmentNameRE = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}-[a-z0-9]+(?:-[a-z0-9]+)*\.md$`)

// FragmentStampFormat is the layout a fragment name starts with. Format it in
// UTC: fragments written on machines in different timezones have to sort into
// the order they were written.
const FragmentStampFormat = "20060102T150405"

func FragmentName(stamp, slug string) string {
	return fmt.Sprintf("%s-%s.md", stamp, slug)
}

type fragmentHeader struct {
	Kind string `yaml:"kind"`
}

// parseFragment reads one unreleased change. The filename carries nothing but
// ordering and uniqueness; everything the renderer needs is in the header.
func parseFragment(filename string, raw []byte) (Fragment, error) {
	if err := CheckFragmentName(filename); err != nil {
		return Fragment{}, fmt.Errorf(
			"changelog %s/%s: %w; write one with `mise run changelog:new`", UnreleasedDir, filename, err)
	}

	header, body, err := splitFrontmatter(raw)
	if err != nil {
		return Fragment{}, fmt.Errorf("changelog %s/%s: %w", UnreleasedDir, filename, err)
	}
	var fm fragmentHeader
	if err := yaml.Unmarshal([]byte(header), &fm); err != nil {
		return Fragment{}, fmt.Errorf("changelog %s/%s: parse frontmatter: %w", UnreleasedDir, filename, err)
	}
	kind, ok := ParseKind(fm.Kind)
	if !ok {
		return Fragment{}, fmt.Errorf("changelog %s/%s: kind %q is not one of %v", UnreleasedDir, filename, fm.Kind, Kinds)
	}

	body = strings.TrimSpace(body)
	if body == "" {
		return Fragment{}, fmt.Errorf("changelog %s/%s: has no body", UnreleasedDir, filename)
	}
	return Fragment{Name: filename, Kind: kind, Body: body}, nil
}

// renderDraft assembles fragments into the markdown the draft entry carries.
// The result is the same shape as a promoted release entry's body, which is
// what lets promotion move it unchanged.
//
// Bullets are written adjacent, with no blank line between them, because a
// blank line makes the list loose and the app then renders every item with its
// own paragraph spacing.
func renderDraft(fragments []Fragment) string {
	var out strings.Builder
	for _, kind := range Kinds {
		section := make([]Fragment, 0, len(fragments))
		for _, fragment := range fragments {
			if fragment.Kind == kind {
				section = append(section, fragment)
			}
		}
		if len(section) == 0 {
			continue
		}
		slices.SortFunc(section, func(x, y Fragment) int { return strings.Compare(x.Name, y.Name) })

		if out.Len() > 0 {
			out.WriteString("\n")
		}
		fmt.Fprintf(&out, "## %s\n\n", kind.heading())
		for _, fragment := range section {
			out.WriteString(bullet(fragment.Body) + "\n")
		}
	}
	return strings.TrimSpace(out.String())
}

// bullet turns a fragment body into one list item, indenting continuation
// lines so a wrapped or multi-paragraph note stays inside its bullet.
func bullet(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		switch {
		case i == 0:
			lines[i] = "- " + line
		case strings.TrimSpace(line) == "":
			lines[i] = ""
		default:
			lines[i] = "  " + line
		}
	}
	return strings.Join(lines, "\n")
}

// CheckFragmentName reports whether name is one this package will read back,
// so a tool that builds a name can assert it against the parser rather than
// against a copy of the pattern.
func CheckFragmentName(name string) error {
	if !fragmentNameRE.MatchString(name) {
		return fmt.Errorf("%q is not <YYYYMMDDThhmmss>-<slug>.md", name)
	}
	return nil
}
