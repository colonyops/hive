package main

import (
	"strings"
	"testing"
)

func TestValidatePrepareSourceState(t *testing.T) {
	t.Parallel()

	valid := releaseSourceState{branch: "main", head: "abc", originMain: "abc"}
	if err := validatePrepareSourceState(valid); err != nil {
		t.Fatalf("valid source rejected: %v", err)
	}

	tests := []struct {
		name  string
		state releaseSourceState
		want  string
	}{
		{name: "dirty", state: releaseSourceState{branch: "main", head: "abc", originMain: "abc", dirty: true}, want: "not clean"},
		{name: "behind", state: releaseSourceState{branch: "main", head: "abc", originMain: "def"}, want: "does not equal"},
		{name: "feature branch", state: releaseSourceState{branch: "feat/release", head: "abc", originMain: "abc"}, want: "want main"},
		{name: "detached", state: releaseSourceState{head: "abc", originMain: "abc"}, want: "detached HEAD"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validatePrepareSourceState(test.state)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validatePrepareSourceState() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidatePublishSourceState(t *testing.T) {
	t.Parallel()

	const tag = "desktop-v1.2.3-dev.1"
	valid := []releaseSourceState{
		{branch: "main", head: "abc", originMain: "abc"},
		{head: "abc", originMain: "abc", headOnOriginMain: true, tagsAtHead: []string{tag}},
		{head: "abc", originMain: "def", headOnOriginMain: true, tagsAtHead: []string{tag}},
	}
	for _, state := range valid {
		if err := validatePublishSourceState(state, tag); err != nil {
			t.Fatalf("valid source rejected: %v", err)
		}
	}

	tests := []struct {
		name  string
		state releaseSourceState
		want  string
	}{
		{name: "dirty", state: releaseSourceState{branch: "main", head: "abc", originMain: "abc", dirty: true}, want: "not clean"},
		{name: "not current main", state: releaseSourceState{branch: "main", head: "abc", originMain: "def"}, want: "does not equal"},
		{name: "feature branch", state: releaseSourceState{branch: "feat/release", head: "abc", originMain: "abc"}, want: "want main"},
		{name: "wrong detached tag", state: releaseSourceState{head: "abc", originMain: "abc", headOnOriginMain: true, tagsAtHead: []string{"desktop-v1.2.3-dev.2"}}, want: "not tagged"},
		{name: "tag not on main", state: releaseSourceState{head: "abc", originMain: "def", tagsAtHead: []string{tag}}, want: "not on origin/main"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validatePublishSourceState(test.state, tag)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validatePublishSourceState() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
