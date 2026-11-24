// Package tagset holds the rules every tagged record in hive shares: what a
// valid tag is, and how a list of tags is normalized, matched and edited.
// Tags compare case-insensitively, and the first spelling wins.
package tagset

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxLength keeps a tag short enough to show as a chip beside a name.
const MaxLength = 40

// ErrInvalid wraps every error Validate returns.
var ErrInvalid = errors.New("invalid tag")

// Validate checks one tag. A comma is refused because editors and command
// lines use it to separate tags.
func Validate(tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return fmt.Errorf("%w: it is empty", ErrInvalid)
	}
	if n := utf8.RuneCountInString(tag); n > MaxLength {
		return fmt.Errorf("%w: %q is %d characters; the maximum is %d", ErrInvalid, tag, n, MaxLength)
	}
	if strings.ContainsRune(tag, ',') || strings.ContainsFunc(tag, unicode.IsControl) {
		return fmt.Errorf("%w: %q contains a comma or a control character", ErrInvalid, tag)
	}
	return nil
}

// Normalize trims each tag and drops empty ones and repeats.
func Normalize(tags []string) ([]string, error) {
	var out []string
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if err := Validate(tag); err != nil {
			return nil, err
		}
		if !Contains(out, tag) {
			out = append(out, tag)
		}
	}
	return out, nil
}

// Parse splits a comma-separated list, as typed into an editor or a flag,
// and normalizes it.
func Parse(s string) ([]string, error) {
	return Normalize(strings.Split(s, ","))
}

// Contains reports whether tags holds tag, ignoring case.
func Contains(tags []string, tag string) bool {
	tag = strings.TrimSpace(tag)
	return slices.ContainsFunc(tags, func(t string) bool {
		return strings.EqualFold(strings.TrimSpace(t), tag)
	})
}

// ContainsAll reports whether tags holds every tag in required.
func ContainsAll(tags, required []string) bool {
	for _, tag := range required {
		if !Contains(tags, tag) {
			return false
		}
	}
	return true
}

// Edit adds and then removes tags, and returns the result without changing
// tags. Removing a tag that is not there is not an error. Only the added tags
// are validated, so a tag stored before validation existed can still be
// removed.
func Edit(tags, add, remove []string) ([]string, error) {
	added, err := Normalize(add)
	if err != nil {
		return nil, err
	}
	out := slices.Clone(tags)
	for _, tag := range added {
		if !Contains(out, tag) {
			out = append(out, tag)
		}
	}
	return slices.DeleteFunc(out, func(tag string) bool {
		return Contains(remove, tag)
	}), nil
}
