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

func TestPoliciesFirstMatchWins(t *testing.T) {
	p := core.NewPolicies([]config.Policy{
		policy("staff", "*@corp.example"),
		policy("everyone", "*"),
	})

	got, ok := p.Match("dev@corp.example")
	require.True(t, ok)
	assert.Equal(t, "staff", got.Name)

	got, ok = p.Match("dev@other.example")
	require.True(t, ok)
	assert.Equal(t, "everyone", got.Name)
}

func TestPoliciesMatchIgnoresCase(t *testing.T) {
	p := core.NewPolicies([]config.Policy{policy("staff", "*@Corp.Example")})

	got, ok := p.Match("  Dev@CORP.example ")
	require.True(t, ok)
	assert.Equal(t, "staff", got.Name)
}

func TestPoliciesNoMatch(t *testing.T) {
	p := core.NewPolicies([]config.Policy{policy("staff", "*@corp.example")})

	_, ok := p.Match("dev@other.example")
	assert.False(t, ok)
}

func TestPoliciesEmptyListAdmitsNobody(t *testing.T) {
	_, ok := core.NewPolicies(nil).Match("dev@example.com")
	assert.False(t, ok)
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
