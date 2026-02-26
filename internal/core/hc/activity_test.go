package hc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestActivityTypeIsValid(t *testing.T) {
	tests := []struct {
		name     string
		actType  ActivityType
		expected bool
	}{
		{"update is valid", ActivityTypeUpdate, true},
		{"comment is valid", ActivityTypeComment, true},
		{"checkpoint is valid", ActivityTypeCheckpoint, true},
		{"status_change is valid", ActivityTypeStatusChange, true},
		{"bad is invalid", ActivityType("bad"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.actType.IsValid())
		})
	}
}
