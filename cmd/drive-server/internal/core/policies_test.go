package core_test

import (
	"fmt"
	"sync"
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

func TestPoliciesSwapRacingMatch(t *testing.T) {
	p := core.NewPolicies([]config.Policy{policy("a", "*")})

	var wg sync.WaitGroup
	for i := range 4 {
		wg.Go(func() {
			for j := range 200 {
				if i%2 == 0 {
					p.Swap([]config.Policy{policy(fmt.Sprintf("p%d", j), "*")})
				} else {
					_, ok := p.Match("dev@example.com")
					assert.True(t, ok)
				}
			}
		})
	}
	wg.Wait()
}
