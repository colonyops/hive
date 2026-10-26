package session

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxTagLength keeps a tag short enough to show as a chip beside a
// session name.
const MaxTagLength = 40

// ValidateTag checks one tag. A comma is refused because editors and
// command lines use it to separate tags.
func ValidateTag(tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return fmt.Errorf("tag cannot be empty")
	}
	if n := utf8.RuneCountInString(tag); n > MaxTagLength {
		return fmt.Errorf("tag %q is %d characters; the maximum is %d", tag, n, MaxTagLength)
	}
	if strings.ContainsRune(tag, ',') || strings.ContainsFunc(tag, unicode.IsControl) {
		return fmt.Errorf("tag %q cannot contain commas or control characters", tag)
	}
	return nil
}

// NormalizeTags trims each tag and drops empty ones and repeats. Tags
// compare case-insensitively, and the first spelling wins.
func NormalizeTags(tags []string) ([]string, error) {
	var out []string
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if err := ValidateTag(tag); err != nil {
			return nil, err
		}
		if !containsTag(out, tag) {
			out = append(out, tag)
		}
	}
	return out, nil
}

// ParseTags splits a comma-separated list, as typed into an editor or a
// flag, and normalizes it.
func ParseTags(s string) ([]string, error) {
	return NormalizeTags(strings.Split(s, ","))
}

// HasTag reports whether the session carries tag, ignoring case.
func (s *Session) HasTag(tag string) bool {
	return containsTag(s.Tags, strings.TrimSpace(tag))
}

// HasAllTags reports whether the session carries every tag in required.
func (s *Session) HasAllTags(required []string) bool {
	for _, tag := range required {
		if !s.HasTag(tag) {
			return false
		}
	}
	return true
}

// EditTags adds and then removes tags. Removing a tag the session does
// not carry is not an error.
func (s *Session) EditTags(add, remove []string) error {
	added, err := NormalizeTags(append(slices.Clone(s.Tags), add...))
	if err != nil {
		return err
	}
	s.Tags = slices.DeleteFunc(added, func(tag string) bool {
		return containsTag(remove, tag)
	})
	return nil
}

func containsTag(tags []string, tag string) bool {
	return slices.ContainsFunc(tags, func(l string) bool {
		return strings.EqualFold(strings.TrimSpace(l), tag)
	})
}
