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

// MaxNameLength keeps a long title from producing an unwieldy tmux name or path.
const MaxNameLength = 60

var (
	nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

	validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 _.:/\-]*$`)

	// Letters that Unicode does not decompose into an ASCII base and a mark.
	foldLetters = strings.NewReplacer("ß", "ss", "æ", "ae", "œ", "oe", "ø", "o", "ł", "l", "đ", "d", "þ", "th")
)

// ValidateName checks a name's length and characters, not its uniqueness.
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

// Slugify lowercases name, folds accents, and joins the remaining letters and
// digits with single hyphens: "Café/Menu" -> "cafe-menu".
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = foldLetters.Replace(s)
	if folded, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn))), s); err == nil {
		s = folded
	}
	s = nonAlphanumeric.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// ToSessionName turns generated text into a valid, length-capped slug, trying
// each fallback when raw has no letters or digits. It returns "" if none does.
func ToSessionName(raw string, fallbacks ...string) string {
	for _, candidate := range append([]string{raw}, fallbacks...) {
		if slug := Slugify(candidate); slug != "" {
			return truncateName(slug, MaxNameLength)
		}
	}
	return ""
}

// NameWithSuffix appends "-suffix", shortening name to stay within MaxNameLength.
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
