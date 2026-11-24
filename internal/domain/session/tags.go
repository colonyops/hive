package session

import "github.com/colonyops/hive/internal/domain/tagset"

// HasTag reports whether the session carries tag, ignoring case.
func (s *Session) HasTag(tag string) bool {
	return tagset.Contains(s.Tags, tag)
}

// HasAllTags reports whether the session carries every tag in required.
func (s *Session) HasAllTags(required []string) bool {
	return tagset.ContainsAll(s.Tags, required)
}

// EditTags adds and then removes tags; see tagset.Edit.
func (s *Session) EditTags(add, remove []string) error {
	tags, err := tagset.Edit(s.Tags, add, remove)
	if err != nil {
		return err
	}
	s.Tags = tags
	return nil
}
