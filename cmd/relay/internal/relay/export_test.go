package relay

import "time"

func (a *Auth) SetNow(now func() time.Time) { a.now = now }

func (s *Sweeper) SetNow(now func() time.Time) { s.now = now }

func (s *SignIn) SetNow(now func() time.Time) {
	s.now = now
	s.auth.now = now
}

var Seal = seal
