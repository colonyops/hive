package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
)

func policy(name string, globs ...string) config.Policy {
	return config.Policy{Name: name, Matches: globs}
}

func TestPoliciesMatch(t *testing.T) {
	p := core.NewPolicies([]config.Policy{
		policy("staff", "*@Corp.Example"),
		policy("everyone", "*@open.example"),
		policy("catchall-too", "*"),
	})
	tests := []struct {
		email string
		want  string
	}{
		{"dev@corp.example", "staff"},
		{"  Dev@CORP.example ", "staff"},
		{"dev@open.example", "everyone"},
		{"dev@other.example", "catchall-too"},
	}
	for _, tt := range tests {
		got, ok := p.Match(tt.email)
		require.True(t, ok, tt.email)
		assert.Equal(t, tt.want, got.Name, tt.email)
	}

	_, ok := core.NewPolicies([]config.Policy{policy("staff", "*@corp.example")}).Match("dev@other.example")
	assert.False(t, ok)
	_, ok = core.NewPolicies(nil).Match("dev@example.com")
	assert.False(t, ok, "an empty list admits nobody")
}
