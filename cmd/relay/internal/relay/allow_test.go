package relay_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

func TestAllowlist(t *testing.T) {
	a := relay.NewAllowlist([]string{"*@colonyops.dev", "Hayden@Example.com"}, []string{"pw-user@elsewhere.org"})
	tests := []struct {
		email string
		want  bool
	}{
		{"anyone@colonyops.dev", true},
		{"ANYONE@ColonyOps.dev", true},
		{"hayden@example.com", true},
		{"  HAYDEN@example.com ", true},
		{"other@example.com", false},
		{"someone@colonyops.dev.evil.com", false},
		{"pw-user@elsewhere.org", true},
		{"", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, a.Allow(tt.email), tt.email)
	}
}

func TestAllowlistEmptyAdmitsNobody(t *testing.T) {
	a := relay.NewAllowlist(nil, nil)
	assert.False(t, a.Allow("a@example.com"))
	assert.False(t, a.HasPatterns())
}

func TestAllowlistReload(t *testing.T) {
	a := relay.NewAllowlist([]string{"a@example.com"}, nil)
	assert.True(t, a.Allow("a@example.com"))
	a.Reload([]string{"b@example.com"}, nil)
	assert.False(t, a.Allow("a@example.com"))
	assert.True(t, a.Allow("b@example.com"))

	a.Reload(nil, []string{"c@example.com"})
	assert.True(t, a.Allow("c@example.com"))
	assert.False(t, a.HasPatterns(), "users alone are not patterns")
}
