package core

import (
	"path"
	"strings"
	"sync/atomic"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
)

// Policies is the set of user policies. Swap lets tests change them on a
// running server, as a restart with a new config would.
type Policies struct {
	cur atomic.Pointer[[]config.Policy]
}

func NewPolicies(ps []config.Policy) *Policies {
	p := &Policies{}
	p.Swap(ps)
	return p
}

func (p *Policies) Swap(ps []config.Policy) {
	ps = append([]config.Policy(nil), ps...)
	p.cur.Store(&ps)
}

// Match returns the first policy with a glob that matches email. An empty
// policy list admits nobody.
func (p *Policies) Match(email string) (config.Policy, bool) {
	email = NormalizeEmail(email)
	for _, pol := range *p.cur.Load() {
		for _, glob := range pol.Matches {
			if ok, err := path.Match(strings.ToLower(strings.TrimSpace(glob)), email); err == nil && ok {
				return pol, true
			}
		}
	}
	return config.Policy{}, false
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
