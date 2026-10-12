// Package memory is the in-memory relay.Store: maps under one mutex. It is
// the reference driver the contract test holds every other driver to.
package memory

import (
	"cmp"
	"context"
	"crypto/subtle"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

type store struct {
	mu       sync.Mutex
	accounts map[string]relay.Account
	tokens   map[string]relay.Token
	sources  map[string]relay.Source
	// deliveries holds each source's rows sorted by ID ascending.
	deliveries map[string][]relay.Delivery
	codes      map[string]authCode
	outbox     map[string]relay.Mail
	recipients map[string]relay.Recipient
}

type authCode struct {
	hash     string
	expires  time.Time
	attempts int
}

func New() relay.Store {
	return &store{
		accounts:   map[string]relay.Account{},
		tokens:     map[string]relay.Token{},
		sources:    map[string]relay.Source{},
		deliveries: map[string][]relay.Delivery{},
		codes:      map[string]authCode{},
		outbox:     map[string]relay.Mail{},
		recipients: map[string]relay.Recipient{},
	}
}

func (s *store) Accounts() relay.AccountStore     { return accounts{s} }
func (s *store) Tokens() relay.TokenStore         { return tokens{s} }
func (s *store) Sources() relay.SourceStore       { return sources{s} }
func (s *store) Deliveries() relay.DeliveryStore  { return deliveries{s} }
func (s *store) AuthCodes() relay.AuthCodeStore   { return authCodes{s} }
func (s *store) Outbox() relay.OutboxStore        { return outbox{s} }
func (s *store) Recipients() relay.RecipientStore { return recipients{s} }
func (s *store) Close() error                     { return nil }

func copyDelivery(d relay.Delivery) relay.Delivery {
	d.Record = slices.Clone(d.Record)
	return d
}

func (s *store) deleteSourceLocked(src relay.Source) {
	var freed int64
	for _, d := range s.deliveries[src.ID] {
		freed += d.Size
	}
	delete(s.deliveries, src.ID)
	delete(s.sources, src.ID)
	s.creditLocked(src.Email, freed)
}

func (s *store) creditLocked(email string, bytes int64) {
	if a, ok := s.accounts[email]; ok {
		a.BytesUsed -= bytes
		s.accounts[email] = a
	}
}

func (s *store) ownedSourceLocked(email, id string) (relay.Source, error) {
	src, ok := s.sources[id]
	if !ok || src.Email != email {
		return relay.Source{}, relay.ErrNotFound
	}
	return src, nil
}

type accounts struct{ s *store }

func (a accounts) Get(_ context.Context, email string) (relay.Account, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	acct, ok := a.s.accounts[email]
	if !ok {
		return relay.Account{}, relay.ErrNotFound
	}
	return acct, nil
}

func (a accounts) Put(_ context.Context, acct relay.Account) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	if existing, ok := a.s.accounts[acct.Email]; ok {
		existing.Encryption = acct.Encryption
		acct = existing
	}
	a.s.accounts[acct.Email] = acct
	return nil
}

func (a accounts) Delete(_ context.Context, email string) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	if _, ok := a.s.accounts[email]; !ok {
		return relay.ErrNotFound
	}
	for id, t := range a.s.tokens {
		if t.Email == email {
			delete(a.s.tokens, id)
		}
	}
	for id, src := range a.s.sources {
		if src.Email == email {
			delete(a.s.deliveries, id)
			delete(a.s.sources, id)
		}
	}
	for id, r := range a.s.recipients {
		if r.Email == email {
			delete(a.s.recipients, id)
		}
	}
	delete(a.s.accounts, email)
	return nil
}

func (a accounts) List(_ context.Context) ([]relay.Account, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	out := make([]relay.Account, 0, len(a.s.accounts))
	for _, acct := range a.s.accounts {
		out = append(out, acct)
	}
	slices.SortFunc(out, func(x, y relay.Account) int { return cmp.Compare(x.Email, y.Email) })
	return out, nil
}

func (a accounts) AddBytes(_ context.Context, email string, delta int64) (int64, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	acct, ok := a.s.accounts[email]
	if !ok {
		return 0, relay.ErrNotFound
	}
	acct.BytesUsed += delta
	a.s.accounts[email] = acct
	return acct.BytesUsed, nil
}

type tokens struct{ s *store }

func (t tokens) GetByHash(_ context.Context, hash string) (relay.Token, error) {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	for _, tok := range t.s.tokens {
		if tok.Hash == hash {
			return tok, nil
		}
	}
	return relay.Token{}, relay.ErrNotFound
}

func (t tokens) Put(_ context.Context, tok relay.Token) error {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	t.s.tokens[tok.ID] = tok
	return nil
}

func (t tokens) Delete(_ context.Context, email, id string) error {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	tok, ok := t.s.tokens[id]
	if !ok || tok.Email != email {
		return relay.ErrNotFound
	}
	delete(t.s.tokens, id)
	return nil
}

func (t tokens) ListByEmail(_ context.Context, email string) ([]relay.Token, error) {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	var out []relay.Token
	for _, tok := range t.s.tokens {
		if tok.Email == email {
			out = append(out, tok)
		}
	}
	slices.SortFunc(out, func(x, y relay.Token) int {
		return cmp.Or(x.CreatedAt.Compare(y.CreatedAt), cmp.Compare(x.ID, y.ID))
	})
	return out, nil
}

func (t tokens) Touch(_ context.Context, id string, at time.Time) error {
	t.s.mu.Lock()
	defer t.s.mu.Unlock()
	tok, ok := t.s.tokens[id]
	if !ok {
		return relay.ErrNotFound
	}
	tok.LastUsedAt = at
	t.s.tokens[id] = tok
	return nil
}

type sources struct{ s *store }

func (r sources) Get(_ context.Context, email, id string) (relay.Source, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	return r.s.ownedSourceLocked(email, id)
}

func (r sources) GetByIngressHash(_ context.Context, hash string) (relay.Source, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, src := range r.s.sources {
		if src.IngressHash == hash {
			return src, nil
		}
	}
	return relay.Source{}, relay.ErrNotFound
}

func (r sources) Put(_ context.Context, src relay.Source) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.sources[src.ID] = src
	return nil
}

func (r sources) Delete(_ context.Context, email, id string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	src, err := r.s.ownedSourceLocked(email, id)
	if err != nil {
		return err
	}
	r.s.deleteSourceLocked(src)
	return nil
}

func (r sources) ListByEmail(_ context.Context, email string) ([]relay.Source, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []relay.Source
	for _, src := range r.s.sources {
		if src.Email == email {
			out = append(out, src)
		}
	}
	slices.SortFunc(out, func(x, y relay.Source) int {
		return cmp.Or(x.CreatedAt.Compare(y.CreatedAt), cmp.Compare(x.ID, y.ID))
	})
	return out, nil
}

func (r sources) CountByEmail(_ context.Context, email string) (int, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	n := 0
	for _, src := range r.s.sources {
		if src.Email == email {
			n++
		}
	}
	return n, nil
}

type deliveries struct{ s *store }

func (r deliveries) Append(_ context.Context, email string, d relay.Delivery, maxAccountBytes int64, keepPerSource int) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	acct, ok := r.s.accounts[email]
	if !ok {
		return relay.ErrNotFound
	}
	if _, err := r.s.ownedSourceLocked(email, d.SourceID); err != nil {
		return err
	}
	if acct.BytesUsed+d.Size > maxAccountBytes {
		return &relay.Error{
			Kind:    relay.KindOverCap,
			Message: fmt.Sprintf("account is at its cap of %d bytes", maxAccountBytes),
		}
	}

	rows := r.s.deliveries[d.SourceID]
	i, _ := slices.BinarySearchFunc(rows, d.ID, func(x relay.Delivery, id string) int { return cmp.Compare(x.ID, id) })
	rows = slices.Insert(rows, i, copyDelivery(d))
	acct.BytesUsed += d.Size
	r.s.accounts[email] = acct

	if keepPerSource > 0 && len(rows) > keepPerSource {
		var freed int64
		cut := len(rows) - keepPerSource
		for _, old := range rows[:cut] {
			freed += old.Size
		}
		rows = slices.Clone(rows[cut:])
		r.s.creditLocked(email, freed)
	}
	r.s.deliveries[d.SourceID] = rows
	return nil
}

func (r deliveries) ListAfter(_ context.Context, sourceID, afterID string, limit int) ([]relay.Delivery, bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	rows := r.s.deliveries[sourceID]
	start, found := slices.BinarySearchFunc(rows, afterID, func(x relay.Delivery, id string) int { return cmp.Compare(x.ID, id) })
	if found {
		start++
	}
	rest := rows[start:]
	more := false
	if limit >= 0 && len(rest) > limit {
		rest, more = rest[:limit], true
	}
	out := make([]relay.Delivery, len(rest))
	for i, d := range rest {
		out[i] = copyDelivery(d)
	}
	return out, more, nil
}

func (r deliveries) DeleteBefore(_ context.Context, email, sourceID string, before time.Time) (int, int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, err := r.s.ownedSourceLocked(email, sourceID); err != nil {
		return 0, 0, err
	}
	var (
		n     int
		freed int64
	)
	r.s.deliveries[sourceID] = slices.DeleteFunc(r.s.deliveries[sourceID], func(d relay.Delivery) bool {
		if d.ReceivedAt.Before(before) {
			n++
			freed += d.Size
			return true
		}
		return false
	})
	r.s.creditLocked(email, freed)
	return n, freed, nil
}

func (r deliveries) TrimTo(_ context.Context, email, sourceID string, keep int) (int, int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, err := r.s.ownedSourceLocked(email, sourceID); err != nil {
		return 0, 0, err
	}
	rows := r.s.deliveries[sourceID]
	keep = max(keep, 0)
	if len(rows) <= keep {
		return 0, 0, nil
	}
	cut := len(rows) - keep
	var freed int64
	for _, d := range rows[:cut] {
		freed += d.Size
	}
	r.s.deliveries[sourceID] = slices.Clone(rows[cut:])
	r.s.creditLocked(email, freed)
	return cut, freed, nil
}

func (r deliveries) Stats(_ context.Context, sourceID string) (int, int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	rows := r.s.deliveries[sourceID]
	var bytes int64
	for _, d := range rows {
		bytes += d.Size
	}
	return len(rows), bytes, nil
}

type authCodes struct{ s *store }

func (a authCodes) Put(_ context.Context, email, hash string, expires time.Time) error {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	a.s.codes[email] = authCode{hash: hash, expires: expires}
	return nil
}

func (a authCodes) Consume(_ context.Context, email, hash string, now time.Time, maxAttempts int) (bool, int, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	c, ok := a.s.codes[email]
	if !ok {
		return false, 0, nil
	}
	if !now.Before(c.expires) {
		delete(a.s.codes, email)
		return false, 0, nil
	}
	if subtle.ConstantTimeCompare([]byte(c.hash), []byte(hash)) == 1 {
		delete(a.s.codes, email)
		return true, 0, nil
	}
	c.attempts++
	left := maxAttempts - c.attempts
	if left <= 0 {
		delete(a.s.codes, email)
		return false, 0, nil
	}
	a.s.codes[email] = c
	return false, left, nil
}

func (a authCodes) DeleteExpired(_ context.Context, now time.Time) (int, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	n := 0
	for email, c := range a.s.codes {
		if !now.Before(c.expires) {
			delete(a.s.codes, email)
			n++
		}
	}
	return n, nil
}

type outbox struct{ s *store }

func (o outbox) Append(_ context.Context, m relay.Mail) error {
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	o.s.outbox[m.ID] = m
	return nil
}

func (o outbox) Due(_ context.Context, now time.Time, limit int) ([]relay.Mail, error) {
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	var out []relay.Mail
	for _, m := range o.s.outbox {
		if m.SentAt.IsZero() && !m.NextAt.After(now) {
			out = append(out, m)
		}
	}
	slices.SortFunc(out, func(x, y relay.Mail) int {
		return cmp.Or(x.CreatedAt.Compare(y.CreatedAt), cmp.Compare(x.ID, y.ID))
	})
	if limit >= 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (o outbox) MarkSent(_ context.Context, id string, at time.Time) error {
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	m, ok := o.s.outbox[id]
	if !ok {
		return relay.ErrNotFound
	}
	m.SentAt = at
	o.s.outbox[id] = m
	return nil
}

func (o outbox) MarkFailed(_ context.Context, id string, next time.Time) error {
	o.s.mu.Lock()
	defer o.s.mu.Unlock()
	m, ok := o.s.outbox[id]
	if !ok {
		return relay.ErrNotFound
	}
	m.NextAt = next
	m.Attempts++
	o.s.outbox[id] = m
	return nil
}

type recipients struct{ s *store }

func (r recipients) Put(_ context.Context, rec relay.Recipient) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for id, existing := range r.s.recipients {
		if existing.Email == rec.Email && existing.Recipient == rec.Recipient {
			existing.Device = rec.Device
			r.s.recipients[id] = existing
			return nil
		}
	}
	r.s.recipients[rec.ID] = rec
	return nil
}

func (r recipients) Delete(_ context.Context, email, id string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	rec, ok := r.s.recipients[id]
	if !ok || rec.Email != email {
		return relay.ErrNotFound
	}
	delete(r.s.recipients, id)
	return nil
}

func (r recipients) ListByEmail(_ context.Context, email string) ([]relay.Recipient, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []relay.Recipient
	for _, rec := range r.s.recipients {
		if rec.Email == email {
			out = append(out, rec)
		}
	}
	slices.SortFunc(out, func(x, y relay.Recipient) int {
		return cmp.Or(x.CreatedAt.Compare(y.CreatedAt), cmp.Compare(x.ID, y.ID))
	})
	return out, nil
}
