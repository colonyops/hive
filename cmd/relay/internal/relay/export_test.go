package relay

import "time"

func (a *Auth) SetNow(now func() time.Time) { a.now = now }

func (i *Ingress) SetNow(now func() time.Time) { i.now = now }

func (s *Sweeper) SetNow(now func() time.Time) { s.now = now }
