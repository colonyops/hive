package session

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxLabelLength keeps a label short enough to show as a chip beside a
// session name.
const MaxLabelLength = 40

// ValidateLabel checks one label. A comma is refused because editors and
// command lines use it to separate labels.
func ValidateLabel(label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return fmt.Errorf("label cannot be empty")
	}
	if n := utf8.RuneCountInString(label); n > MaxLabelLength {
		return fmt.Errorf("label %q is %d characters; the maximum is %d", label, n, MaxLabelLength)
	}
	if strings.ContainsRune(label, ',') || strings.ContainsFunc(label, unicode.IsControl) {
		return fmt.Errorf("label %q cannot contain commas or control characters", label)
	}
	return nil
}

// NormalizeLabels trims each label and drops empty ones and repeats. Labels
// compare case-insensitively, and the first spelling wins.
func NormalizeLabels(labels []string) ([]string, error) {
	var out []string
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if err := ValidateLabel(label); err != nil {
			return nil, err
		}
		if !containsLabel(out, label) {
			out = append(out, label)
		}
	}
	return out, nil
}

// ParseLabels splits a comma-separated list, as typed into an editor or a
// flag, and normalizes it.
func ParseLabels(s string) ([]string, error) {
	return NormalizeLabels(strings.Split(s, ","))
}

// HasLabel reports whether the session carries label, ignoring case.
func (s *Session) HasLabel(label string) bool {
	return containsLabel(s.Labels, strings.TrimSpace(label))
}

// HasAllLabels reports whether the session carries every label in required.
func (s *Session) HasAllLabels(required []string) bool {
	for _, label := range required {
		if !s.HasLabel(label) {
			return false
		}
	}
	return true
}

// EditLabels adds and then removes labels. Removing a label the session does
// not carry is not an error.
func (s *Session) EditLabels(add, remove []string) error {
	added, err := NormalizeLabels(append(slices.Clone(s.Labels), add...))
	if err != nil {
		return err
	}
	s.Labels = slices.DeleteFunc(added, func(label string) bool {
		return containsLabel(remove, label)
	})
	return nil
}

func containsLabel(labels []string, label string) bool {
	return slices.ContainsFunc(labels, func(l string) bool {
		return strings.EqualFold(strings.TrimSpace(l), label)
	})
}
