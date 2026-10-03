package session

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// MaxNameLength caps both a typed Name and a generated one, so a long issue or
// pull request title cannot produce an unwieldy tmux name, branch, or path.
const MaxNameLength = 60

var (
	nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

	// Disallowed: ~ ^ * ? [ \ @ and control characters, which are meaningless
	// or harmful in the branch and ticket names developers type here.
	validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 _.:/\-]*$`)

	// Letters that Unicode does not decompose into an ASCII base and a mark.
	foldLetters = strings.NewReplacer("ß", "ss", "æ", "ae", "œ", "oe", "ø", "o", "ł", "l", "đ", "d", "þ", "th")
)

// ValidateName returns an error if name is blank, longer than MaxNameLength,
// or contains characters outside the allowed set. It does not check
// uniqueness, which depends on the other sessions.
func ValidateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("session name cannot be empty")
	}
	if len(name) > MaxNameLength {
		return fmt.Errorf("session name is %d characters; the maximum is %d", len(name), MaxNameLength)
	}
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid session name: allowed characters are a-z, 0-9, spaces, and - _ : . /")
	}
	return nil
}

// Slugify converts a name to the slug used for tmux session names and
// directory paths: lowercase ASCII letters and digits joined by single hyphens.
// Accented letters fold to their base letter; anything else becomes a hyphen.
//
//	"My Session Name" -> "my-session-name"
//	"Café/Menu"       -> "cafe-menu"
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = foldLetters.Replace(s)
	if folded, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn))), s); err == nil {
		s = folded
	}
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// ToSessionName normalizes generated text, such as a rendered template or an
// issue title, into a name that passes ValidateName. It slugifies raw and caps
// it at MaxNameLength on a word boundary. When raw has no letters or digits,
// it tries each fallback in turn. It returns "" when nothing survives.
func ToSessionName(raw string, fallbacks ...string) string {
	for _, candidate := range append([]string{raw}, fallbacks...) {
		if slug := Slugify(candidate); slug != "" {
			return truncateName(slug, MaxNameLength)
		}
	}
	return ""
}

// NameWithSuffix appends "-suffix" to name, shortening name on a word boundary
// so the result stays within MaxNameLength.
func NameWithSuffix(name, suffix string) string {
	return truncateName(name, MaxNameLength-len(suffix)-1) + "-" + suffix
}

func truncateName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	cut := name[:maxLen]
	if idx := strings.LastIndexAny(cut, "-_ .:/"); idx > 0 {
		cut = cut[:idx]
	}
	return strings.TrimRight(cut, "-_ .:/")
}
